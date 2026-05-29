package rpcclient

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var rpcCodeMessages = map[codes.Code]string{
	codes.PermissionDenied: "permission denied",
	codes.Unauthenticated:  "unauthenticated",
	codes.Unavailable:      "service unavailable",
	codes.DeadlineExceeded: "request timed out",
	codes.InvalidArgument:  "invalid argument",
	codes.NotFound:         "not found",
	codes.AlreadyExists:    "already exists",
}

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	message := st.Message()
	prefix, ok := rpcCodeMessages[st.Code()]
	if !ok {
		prefix = fmt.Sprintf("grpc error %d", st.Code())
	}
	if message == "" {
		return errors.New(prefix)
	}
	return fmt.Errorf("%s: %s", prefix, message)
}
