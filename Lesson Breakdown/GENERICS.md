# Generics

## Best used when you want to have a decoder that works on multiple 
## Client Request struct and their handlers.

---

# Usage
### we will specify a generic via the square bracket beside our function name
```go
// TO SUPPLY THE STRUCT NOW WE WILL CALL IT LIKE:
// decodeJSON[REQUEST STRUCT]
func decodeJSON[T any](r *http.Request) (T, error) {
	var zero T
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&zero); err != nil {
		return zero, fmt.Errorf("error: %w", err)
	}
	
	return zero, nil
}
```

### To use this function:
```go 
decoded, err := decodeJSON[SignupRequest](r)
```

---

# Big picture

```go
package main

import (
	"net/http"
	"encoding/json"
	"fmt"
	)

type SignupRequest struct {
	Username string `json:"username"`
	Level    int    `json:"level"`
}

type PetRequest struct {
	PetName string `json:"pet_name"`
	Species string `json:"species"`
}

// TO SUPPLY THE STRUCT NOW WE WILL CALL IT LIKE:
// decodeJSON[REQUEST STRUCT]
func decodeJSON[T any](r *http.Request) (T, error) {
	var zero T
	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&zero); err != nil {
		return zero, fmt.Errorf("error: %w", err)
	}
	
	return zero, nil
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	decoded, err := decodeJSON[SignupRequest](r)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("error: %w", err)))
	}

	if (decoded.Username == "") || (decoded.Level < 1) {
		w.WriteHeader(400)
		w.Write([]byte("invalid request"))
	}

	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf("welcome %s (level %d)", decoded.Username, decoded.Level)))
}

```