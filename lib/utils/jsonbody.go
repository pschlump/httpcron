package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/pschlump/dbgo"
	"github.com/pschlump/httpcron/lib/config"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
}

func JsonBody(w http.ResponseWriter, r *http.Request, cfg *config.Config, data interface{}) error {

	// Apply Defaults
	err := config.SetDefaults(data)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"status":"error","msg":"Invalid default data in %T"}`, data), http.StatusBadRequest)
		// should log
		dbgo.Fprintf(os.Stderr, "%(cyan)%(LF):%(yellow) Invalid Default \n")
		return fmt.Errorf("Invalid JSON payload")
	}

	// Read the entire body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return err
	}
	r.Body.Close() // Close the original reader

	// Restore r.Body so subsequent handlers can read it
	// r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Use the body (e.g., convert to string or unmarshal JSON)
	bodyString := string(bodyBytes)

	if cfg != nil && cfg.Debug.Enabled["dump-request-body"] {
		dbgo.Printf("Body ->%s<-\n", bodyString)
	}

	// Decode the JSON body into the struct
	// It is more memory-efficient than reading the whole body first
	err = json.Unmarshal(bodyBytes, data)
	if err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		// should log
		if false {
			dbgo.Fprintf(os.Stderr, "%(cyan)%(LF):%(yellow) Invalid Passed Data \n")
		}
		return fmt.Errorf("Invalid JSON payload")
	}

	err = validate.Struct(data)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"status":"error","msg":"Invalid Data: %s"}`, err), http.StatusUnprocessableEntity)
		// should log
		if false {
			dbgo.Fprintf(os.Stderr, "%(cyan)%(LF):%(yellow) Invalid Passed Data - Validate Failed\n")
		}
		return fmt.Errorf("Invalid Data: %s", err)
	}

	return nil
}

/* vim: set noai ts=4 sw=4: */
