package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"go.opentelemetry.io/otel/api/core"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/key"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporters/metric/stdout"
	"go.opentelemetry.io/otel/exporters/trace/zipkin"
	"go.opentelemetry.io/otel/plugin/httptrace"
	"go.opentelemetry.io/otel/plugin/othttp"
	"go.opentelemetry.io/otel/sdk/metric/controller/push"
	"go.opentelemetry.io/otel/sdk/resource/resourcekeys"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	// "go.opentelemetry.io/otel/exporters/otlp"
)

var requestPathKey = key.New("path")

func initTracer() error {
	//exporter, err := otlp.NewExporter(
	//	otlp.WithInsecure(),
	//	otlp.WithAddress(
	//      os.Getenv("SPAN_EXPORTER_HOST") +
	//      ":" +
	//      os.Getenv("SPAN_EXPORTER_PORT")),
	//)
	exporter, err := zipkin.NewExporter(
		os.Getenv("SPAN_EXPORTER_PROTOCOL") +
			"://" +
			os.Getenv("SPAN_EXPORTER_HOST") +
			":" +
			os.Getenv("SPAN_EXPORTER_PORT") +
			os.Getenv("SPAN_EXPORTER_ENDPOINT"))
	if err != nil {
		return err
	}

	tp, err := sdktrace.NewProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResourceAttributes(core.Key(resourcekeys.ServiceKeyName).String("go-service")),
	)
	if err != nil {
		return err
	}

	global.SetTraceProvider(tp)
	return nil
}

func initMeter() *push.Controller {
	pusher, err := stdout.NewExportPipeline(stdout.Config{
		PrettyPrint:    true,
		DoNotPrintTime: true,
	}, time.Second*5)
	if err != nil {
		log.Fatal("Could not initialize stdout exporter:", err)
	}

	return pusher
}

func main() {
	godotenv.Load()

	pusher := initMeter()
	defer pusher.Stop()
	err := initTracer()
	check(err)

	tr := global.Tracer("go-demo/tracer")
	meter := pusher.Meter("go-demo/meter")

	s := &server{
		tracer:          tr,
		requestsCounter: metric.Must(meter).NewInt64Counter("requests-count", metric.WithKeys(requestPathKey)),
	}

	var mux http.ServeMux
	mux.Handle("/", othttp.NewHandler(http.HandlerFunc(s.handler), "hello"))
	fmt.Println("listening on port " + os.Getenv("SERVER_PORT"))
	check(http.ListenAndServe(os.Getenv("SERVER_PORT"), &mux))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type server struct {
	tracer          trace.Tracer
	requestsCounter metric.Int64Counter
}

func (s *server) handler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	s.requestsCounter.Add(ctx, 1, requestPathKey.String(req.URL.Path))

	response := "hello from go\n"
	if pyBody, err := s.fetchFromPythonService(ctx); err == nil {
		response += string(pyBody)
	} else {
		response += "error fetching from python"
	}

	_, _ = io.WriteString(w, strings.Replace(response, "<br>", "\n", -1))
}

func (s *server) fetchFromPythonService(ctx context.Context) ([]byte, error) {
	ctx, span := s.tracer.Start(ctx, "fetch-from-python")
	defer span.End()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	var body []byte

	req, err := http.NewRequest("GET", os.Getenv("PYTHON_ENDPOINT"), nil)
	if err != nil {
		return body, err
	}

	ctx, req = httptrace.W3C(ctx, req)
	httptrace.Inject(ctx, req)

	res, err := client.Do(req)
	if err != nil {
		return body, err
	}
	body, err = ioutil.ReadAll(res.Body)
	err = res.Body.Close()

	return body, err
}
