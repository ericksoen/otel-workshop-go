package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
    godotenv.Load()
	s := &server{}

	var mux http.ServeMux
	mux.Handle("/", http.HandlerFunc(s.handler))
	fmt.Println("listening on port " + os.Getenv("SERVER_PORT"))
	check(http.ListenAndServe(os.Getenv("SERVER_PORT"), &mux))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type server struct{}

func (s *server) handler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	response := "hello from go\n"
	if pyBody, err := s.fetchFromPythonService(ctx); err == nil {
		response += string(pyBody)
	} else {
		response += "error fetching from python"
	}

	_, _ = io.WriteString(w, response)
}

func (s *server) fetchFromPythonService(ctx context.Context) ([]byte, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	var body []byte

	req, err := http.NewRequest("GET", os.Getenv("PYTHON_REMOTE_ENDPOINT"), nil)
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
