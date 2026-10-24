package renderengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"text/template"
	"time"
)

// WorkerArg is the hidden first argument that turns the product binary (or a
// test binary whose TestMain calls RunWorkerIfRequested) into the template
// worker. No other language runtime is involved.
const WorkerArg = "__orchestrator-render-template-worker"

// Worker limits (RB-C04).
const (
	MaxWorkerInput  = 4 << 20
	MaxWorkerOutput = 8 << 20
)

// Stage codes of a template failure. Public errors carry only these codes,
// the bundle-relative file name and a line number.
const (
	CodeTemplateParse   = "TEMPLATE_PARSE"
	CodeTemplateExecute = "TEMPLATE_EXECUTE"
	CodeTemplateOutput  = "TEMPLATE_OUTPUT"
	CodeTemplateTimeout = "TEMPLATE_TIMEOUT"
	CodeWorker          = "TEMPLATE_WORKER"
)

// ErrTemplate is the class of every author template failure.
var ErrTemplate = errors.New("renderengine: template failed")

// TemplateError is a safe template failure: no template value or output.
type TemplateError struct {
	Code string
	File string
	Line int
}

func (e *TemplateError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("renderengine: %s in %s line %d", e.Code, e.File, e.Line)
	}
	return fmt.Sprintf("renderengine: %s in %s", e.Code, e.File)
}
func (e *TemplateError) Is(target error) bool { return target == ErrTemplate }

type workerRequest struct {
	Name   string         `json:"name"`
	Source string         `json:"source"`
	Data   map[string]any `json:"data"`
}

type workerResponse struct {
	Output string `json:"output"`
	Code   string `json:"code,omitempty"`
	Line   int    `json:"line,omitempty"`
}

// RunWorkerIfRequested runs the worker and exits when the process was started
// as one. Call it first in main and in TestMain of packages that render.
func RunWorkerIfRequested() {
	if len(os.Args) == 2 && os.Args[1] == WorkerArg {
		os.Exit(serve(os.Stdin, os.Stdout))
	}
}

func serve(in io.Reader, out io.Writer) int {
	respond := func(r workerResponse) int {
		if err := json.NewEncoder(out).Encode(r); err != nil {
			return 2
		}
		return 0
	}
	raw, err := io.ReadAll(io.LimitReader(in, MaxWorkerInput+1))
	if err != nil || len(raw) > MaxWorkerInput {
		return respond(workerResponse{Code: CodeWorker})
	}
	var req workerRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return respond(workerResponse{Code: CodeWorker})
	}
	tmpl, err := Parse(req.Name, req.Source)
	if err != nil {
		return respond(workerResponse{Code: CodeTemplateParse, Line: line(err)})
	}
	var buffer limitedBuffer
	if err := tmpl.Execute(&buffer, req.Data); err != nil {
		if buffer.exceeded {
			return respond(workerResponse{Code: CodeTemplateOutput})
		}
		return respond(workerResponse{Code: CodeTemplateExecute, Line: line(err)})
	}
	return respond(workerResponse{Output: buffer.String()})
}

// Parse parses one bundle template with the closed helper set. Each file is
// parsed alone: no include can reach outside the parsed bundle file.
func Parse(name, source string) (*template.Template, error) {
	return template.New(name).Option("missingkey=error").Funcs(FuncMap()).Parse(source)
}

var linePattern = regexp.MustCompile(`^template: [^:]*:(\d+)`)

func line(err error) int {
	if m := linePattern.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	limit := b.limit
	if limit == 0 {
		limit = MaxWorkerOutput
	}
	if b.Len()+len(p) > limit {
		b.exceeded = true
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}

// Executor runs author templates in a child process of the product binary
// with bounded JSON stdin/stdout and no inherited environment. Cancelling the
// context kills and reaps the child.
type Executor struct {
	// Command is the worker executable; empty means the running binary.
	Command string
}

func (e Executor) binary() (string, error) {
	if e.Command != "" {
		return e.Command, nil
	}
	return os.Executable()
}

// Execute evaluates the template source of file name with data and returns
// its text output.
func (e Executor) Execute(ctx context.Context, name, source string, data map[string]any) (string, error) {
	request, err := json.Marshal(workerRequest{Name: name, Source: source, Data: data})
	if err != nil {
		return "", &TemplateError{Code: CodeWorker, File: name}
	}
	if len(request) > MaxWorkerInput {
		return "", &TemplateError{Code: CodeTemplateOutput, File: name}
	}
	binary, err := e.binary()
	if err != nil {
		return "", &TemplateError{Code: CodeWorker, File: name}
	}
	cmd := exec.CommandContext(ctx, binary, WorkerArg)
	// No credentials, kubeconfig, Vault token or Git configuration reach the worker.
	cmd.Env = []string{"LANG=C", "LC_ALL=C"}
	cmd.Dir = os.TempDir()
	cmd.Stdin = bytes.NewReader(request)
	// The JSON response escapes the output; allow for that, then check the
	// decoded output against the real limit.
	stdout := limitedBuffer{limit: 2*MaxWorkerOutput + 4096}
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	cmd.WaitDelay = 2 * time.Second
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return "", &TemplateError{Code: CodeTemplateTimeout, File: name}
	}
	if runErr != nil || stdout.exceeded {
		return "", &TemplateError{Code: CodeWorker, File: name}
	}
	var response workerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return "", &TemplateError{Code: CodeWorker, File: name}
	}
	if response.Code != "" {
		return "", &TemplateError{Code: response.Code, File: name, Line: response.Line}
	}
	if len(response.Output) > MaxWorkerOutput {
		return "", &TemplateError{Code: CodeTemplateOutput, File: name}
	}
	return response.Output, nil
}
