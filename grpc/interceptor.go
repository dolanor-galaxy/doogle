package doogle

import (
	"context"
	"path"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func UnaryServerInterceptor(logger *logrus.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var err error
		var reply interface{}
		defer func(begin time.Time) {
			method := path.Base(info.FullMethod)
			fields := logrus.Fields{
				"method":        method,
				"response_time": time.Since(begin),
			}
			if err != nil {
				logger.WithFields(fields).Error("%v", err)
			} else {
				logger.WithFields(fields).Info()
			}
		}(time.Now())
		reply, err = handler(ctx, req)
		return reply, err
	}
}
