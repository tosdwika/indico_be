package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxBody = 1 << 20 // 1MB

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "invalid JSON body: "+err.Error())
		return err
	}
	// reject trailing garbage
	if dec.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "request body must be a single JSON object")
		return errors.New("trailing data")
	}
	return nil
}
