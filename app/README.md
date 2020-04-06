## Go Service

This app listens on port `3000` (443 when accessing from outside glitch) and
exposes a single endpoint at `/` that responds with the string `hello from
go`. For every request it receives, it should call the Python service at
`https://signalfx-otel-workshop-python.glitch.me`.

The following modifications can be made:

* The `SERVER_PORT` can be modified by editing `.env`
* The call destination can be modified by setting  `PYTHON_ENDPOINT` in `.env`

The `.env` file can be used to allow this workshop to be run
in other environments. For example, to run locally, the following changes could
be made:

* In `.env` set the `PYTHON_ENDPOINT` to `http://localhost:3001`

To run in Docker, set `PYTHON_ENDPOINT` to `http://host.docker.internal:3001`

## Running the app

The application is available at
https://glitch.com/edit/#!/signalfx-otel-workshop-go. By default, it runs
an uninstrumented version of the application. From the Glitch site, you
should select the name of the Glitch project (top left) and select `Remix
Project`. You will now have a new Glitch project. The name of the project is
listed in the top left of the window.

To run this workshop locally, you'll need Go 1.13 and Make to be able to run
the service. Run `make run` and then go to http://localhost:3000 to access the
app.

## Instrumenting Go HTTP server and client with OpenTelemetry

Your task is to instrument this application using [OpenTelemetry
Go](https://github.com/open-telemetry/opentelemetry-go). If you get
stuck, check out the `app_instrumented` directory.

### 1. Import packages required for instrumenting our Go app.

```diff
import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"strings"

	"github.com/joho/godotenv"

+	"go.opentelemetry.io/otel/api/core"
+	"go.opentelemetry.io/otel/api/global"
+	"go.opentelemetry.io/otel/api/trace"
+	//"go.opentelemetry.io/otel/exporters/otlp"
+	"go.opentelemetry.io/otel/exporters/trace/zipkin"
+	"go.opentelemetry.io/otel/plugin/httptrace"
+	"go.opentelemetry.io/otel/plugin/othttp"
+	"go.opentelemetry.io/otel/sdk/resource/resourcekeys"
+	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)
```

Note: The recommended deployment model for OpenTelemetry is to have
applications export in OpenTelemetry (OTLP) format to the OpenTelemetry
Collector and have the OpenTelemetry Collector send to your back-end(s) of
choice. OTLP uses gRPC and unfortunately it does not appear Glitch supports
gRPC. As a result, this workshop emits in Zipkin format.

### 2. Configure OpenTelemetry tracer
```diff
	"go.opentelemetry.io/otel/plugin/othttp"
	"go.opentelemetry.io/otel/sdk/resource/resourcekeys"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

+func initTracer() error {
+	//exporter, err := otlp.NewExporter(
+	//	otlp.WithInsecure(),
+	//	otlp.WithAddress(
+	//      os.Getenv("SPAN_EXPORTER_HOST") +
+	//      ":" +
+	//      os.Getenv("SPAN_EXPORTER_PORT")),
+	//)
+	exporter, err := zipkin.NewExporter(
+		os.Getenv("SPAN_EXPORTER_PROTOCOL") +
+		"://" +
+		os.Getenv("SPAN_EXPORTER_HOST") +
+		":" +
+		os.Getenv("SPAN_EXPORTER_PORT") +
+		os.Getenv("SPAN_EXPORTER_ENDPOINT"))
+	if err != nil {
+		return err
+	}
+
+	// configure default trace provider with the service name
+	tp, err := sdktrace.NewProvider(
+		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
+		sdktrace.WithSyncer(exporter),
+		sdktrace.WithResourceAttributes(core.Key(resourcekeys.ServiceKeyName).String("go-service")),
+	)
+	if err != nil {
+		return err
+	}
+
+	// set the configured trace provider as the global provider that other
+	// parts of the app can fetch and use.
+	global.SetTraceProvider(tp)
+	return nil
+}

func main() {
```

Note: You will notice multiple environment variables used above. These
variables should be set in a `.env` file in the same directory as `main.go`.

```bash
SPAN_EXPORTER_HOST=signalfx-otel-workshop-collector.glitch.me
SPAN_EXPORTER_PORT=443
SPAN_EXPORTER_ENDPOINT=/api/v2/spans
SPAN_EXPORTER_PROTOCOL=https
```

### 3. Add a tracer field to the server struct

We'll make the tracer available to all handlers as a field on the server
struct. This is optional and handlers can fetch their own tracers from the
`global` package directly if needed.

#### 3.a Add tracer field to server

```diff
type server struct {
+	tracer trace.Tracer
}
```

#### 3.b Initialize the tracer and add add it to the server instance

```diff
func main() {
	godotenv.Load()

+	err := initTracer()
+	check(err)
+	tr := global.Tracer("go-demo")

-	s := &server{}
+	s := &server{
+		tracer: tr,
+	}

	var mux http.ServeMux
	mux.Handle("/", http.HandlerFunc(s.handler))
	fmt.Println("listening on port " + os.Getenv("SERVER_PORT"))
	check(http.ListenAndServe(os.Getenv("SERVER_PORT"), &mux))
}
```

Note: You will notice an environment variable used above. This
variable should be set in a `.env` file in the same directory as `main.go`.

```bash
SERVER_PORT=3000
```

#### 4. Instrument the HTTP handler

We'll wrap our HTTP handler with the middleware/wrapper provided by the
`othttp` package. The first argument is a handler func while the second is the
operation name.

```diff
func main() {
	godotenv.Load()

	err := initTracer()
	check(err)
	tr := global.Tracer("go-demo")

	s := &server{
		tracer: tr,
	}

	var mux http.ServeMux
-	mux.Handle("/", http.HandlerFunc(s.handler))
+	mux.Handle("/", othttp.NewHandler(http.HandlerFunc(s.handler), "hello"))
	fmt.Println("listening on port " + os.Getenv("SERVER_PORT"))
	check(http.ListenAndServe(os.Getenv("SERVER_PORT"), &mux))
}
```

At this point our app will generate one span for every request it receives.
This will be auto-generated by the `othttp.NewHandler()` wrapper.

#### 5. Add a manual span to record an interesting operation.

```diff
func (s *server) fetchFromPythonService(ctx context.Context) ([]byte, error) {
+	ctx, span := s.tracer.Start(ctx, "fetch-from-python")
+	defer span.End()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	var body []byte

	req, err := http.NewRequest("GET", os.Getenv("PYTHON_ENDPOINT"), nil)
	if err != nil {
		return body, err
	}

	res, err := client.Do(req)
	if err != nil {
		return body, err
	}
	body, err = ioutil.ReadAll(res.Body)
	err = res.Body.Close()

	return body, err
}

```

This will make our app generate a second span with operation name as
`fetch-from-python`. The span will be a child of the previous auto-generated
span.

Note: You will notice an environment variable used above. This
variable should be set in a `.env` file in the same directory as `main.go`.

```bash
PYTHON_REMOTE_ENDPOINT=https://signalfx-otel-workshop-python.glitch.me
```

#### 6. Instrument HTTP client request object to instrument outgoing requests.

Since we are making outgoing calls to an external service, we can wrap our HTTP
request object using the `httptrace` package to auto-generate spans for
operations related to the outgoing calls.

```diff
func (s *server) fetchFromPythonService(ctx context.Context) ([]byte, error) {
	ctx, span := s.tracer.Start(ctx, "fetch-from-python")
	defer span.End()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	var body []byte

	req, err := http.NewRequest("GET", os.Getenv("PYTHON_REMOTE_ENDPOINT"), nil)
	if err != nil {
		return body, err
	}

+	ctx, req = httptrace.W3C(ctx, req)
+	httptrace.Inject(ctx, req)

	res, err := client.Do(req)
	if err != nil {
		return body, err
	}
	body, err = ioutil.ReadAll(res.Body)
	err = res.Body.Close()

	return body, err
}
```

This will generate 4 more spans each representing a sub-operation of the
outgoing HTTP request such as DNS resolution, establishing the connection and
sending the request.

We can run the app again and this time it should emit spans to a locally running
OpenTelemetry Collector.
