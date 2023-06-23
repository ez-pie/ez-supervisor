package signals

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

// 使用无缓冲的通道，利用其阻塞的特性，确保同一时间只有一个信号处理程序能够执行
var onlyOneSignalHandler = make(chan struct{})

// SetupSignalHandler registered for SIGTERM and SIGINT. A context is returned
// which is cancelled on one of these signals. If a second signal is caught,
// the program is terminated with exit code 1.
func SetupSignalHandler() context.Context {
	close(onlyOneSignalHandler) // panics when called twice

	c := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		fmt.Println("<-c 1st")
		cancel()
		<-c
		fmt.Println("<-c 2nd")
		os.Exit(1) // second signal. Exit directly.
	}()

	return ctx
}
