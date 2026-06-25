package archive

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/minio/minio-go/v7"
)

// ErrNotFound indicates the object does not exist in the backing store.
var ErrNotFound = errors.New("archive: object not found")

// IsNotFound reports whether err represents a missing blob/object.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var azErr *azcore.ResponseError
	if errors.As(err, &azErr) && azErr.StatusCode == 404 {
		return true
	}
	var minErr minio.ErrorResponse
	if errors.As(err, &minErr) {
		switch minErr.Code {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	return false
}
