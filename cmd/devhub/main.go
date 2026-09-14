package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	appserver "github.com/Lcrro/devhub/internal/server"
	"github.com/Lcrro/devhub/internal/store"
)

var version = "dev"

func main() {
	port := flag.Int("port", 17890, "local HTTP port")
	dataDir := flag.String("data-dir", "", "override DevHub data directory")
	noOpen := flag.Bool("no-open", false, "do not open the dashboard in a browser")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	st, err := store.New(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	app := appserver.New(st)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil && *port == 17890 {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	srv := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	app.Start(ctx)

	log.Printf("DevHub %s running at %s", version, url)
	log.Printf("Data directory: %s", st.DataDir())
	if !*noOpen {
		go func() { time.Sleep(350 * time.Millisecond); _ = openBrowser(url) }()
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
