package http

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/log"
)

func loggerMiddleware(
	logger *log.Logger,
	accessLevel string,
) func(next http.Handler) http.Handler {

	var loggerfn func(string, ...zap.Field)

	switch accessLevel {
	case zap.DebugLevel.String():
		loggerfn = logger.Debug
	case zap.InfoLevel.String():
		loggerfn = logger.Info
	case zap.WarnLevel.String():
		loggerfn = logger.Warn
	case zap.ErrorLevel.String():
		loggerfn = logger.Error
	default:
		panic(fmt.Sprintf("unsupported access log level: %q", accessLevel))
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			startTime := time.Now()

			defer func() {
				loggerfn("successfully handled HTTP request",
					zap.String("remote_address", r.RemoteAddr),
					zap.String("path", r.URL.Path),
					zap.String("proto", r.Proto),
					zap.String("method", r.Method),
					zap.String("user_agent", r.Header.Get("User-Agent")),
					zap.Int("status", ww.Status()),
					zap.Int64("latency_ns", int64(time.Since(startTime).Nanoseconds())),
					zap.Int("content_in_bytes", contentInBytes(r.Header)),
					zap.Int("content_out_bytes", ww.BytesWritten()),
				)
			}()

			next.ServeHTTP(ww, r)

		}
		return http.HandlerFunc(fn)
	}
}

func contentInBytes(header http.Header) int {
	if i, err := strconv.Atoi(header.Get("Content-Length")); err != nil {
		return 0
	} else {
		return i
	}
}
