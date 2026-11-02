package http

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"orchestrator/internal/platform/sourcezip"
	"orchestrator/internal/ports/moduleregistry"
	"orchestrator/internal/ports/renderregistry"
)

// readSourceUpload streams a strict multipart request into bounded memory; it
// never spills submitted archives to a host temporary directory.
func readSourceUpload(w http.ResponseWriter, r *http.Request, module bool) (map[string]string, []byte, bool) {
	// A context deadline alone cannot interrupt a blocked request-body read.
	// Apply it to the connection while consuming the multipart upload; restore
	// the deadline so a keep-alive connection can handle later requests.
	if deadline, ok := r.Context().Deadline(); ok {
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(deadline); err == nil {
			defer controller.SetReadDeadline(time.Time{})
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, sourcezip.MaxBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		writeCode(w, codeInvalidRequest, "")
		return nil, nil, false
	}
	fields := map[string]string{}
	seen := map[string]bool{}
	var data []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if !writeTooLarge(w, err) {
				writeCode(w, codeInvalidRequest, "")
			}
			return nil, nil, false
		}
		_, params, headerErr := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		name := part.FormName()
		if headerErr != nil || seen[name] || (name != "file" && name != "path" && !(module && name == "resourceType")) {
			writeCode(w, codeInvalidRequest, "")
			return nil, nil, false
		}
		seen[name] = true
		limit := int64(4096)
		if name == "file" {
			limit = sourcezip.MaxBytes
			if !strings.HasSuffix(strings.ToLower(params["filename"]), ".zip") {
				writeCode(w, codeInvalidRequest, "")
				return nil, nil, false
			}
		} else if _, file := params["filename"]; file {
			writeCode(w, codeInvalidRequest, "")
			return nil, nil, false
		}
		value, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			if !writeTooLarge(w, err) {
				writeCode(w, codeInvalidRequest, "")
			}
			return nil, nil, false
		}
		if int64(len(value)) > limit {
			writeTooLarge(w, &http.MaxBytesError{Limit: limit})
			return nil, nil, false
		}
		if name == "file" {
			data = value
		} else {
			fields[name] = strings.TrimSpace(string(value))
		}
	}
	if len(data) == 0 || (module && fields["resourceType"] == "") {
		writeCode(w, codeInvalidRequest, "")
		return nil, nil, false
	}
	return fields, data, true
}

func (s *Server) handleUploadTerraformModule(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), inspectDeadline)
	defer cancel()
	fields, data, ok := readSourceUpload(w, r.WithContext(ctx), true)
	if !ok {
		return
	}
	capability, err := s.catalog.UploadTerraformModule(ctx, org, moduleregistry.ZIPInspection{ResourceType: fields["resourceType"], Path: fields["path"], Data: data})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capability": capability})
}
func (s *Server) handleUploadRenderBundle(w http.ResponseWriter, r *http.Request) {
	org, ok := s.platformActor(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), inspectDeadline)
	defer cancel()
	fields, data, ok := readSourceUpload(w, r.WithContext(ctx), false)
	if !ok {
		return
	}
	bundle, err := s.catalog.UploadRenderBundle(ctx, org, renderregistry.ZIPInspection{Path: fields["path"], Data: data})
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"renderBundle": bundle})
}
