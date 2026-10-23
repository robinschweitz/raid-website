package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
)

func TestDetectPattern(t *testing.T) {
	for _, tc := range []struct {
		name     string
		series   *raidseries.Series
		raids    []raid.Raid
		want     bool
		template string
		nextNum  int
	}{
		{
			name:     "empty_raids",
			raids:    nil,
			want:     false,
			template: "",
			nextNum:  0,
		},
		{
			name: "explicitly_configured_template_is_preserved",
			series: &raidseries.Series{
				Name: "ICC Montagsraid", NameTemplate: "custom-{name}-{number}", NextRaidNumber: 7,
			},
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc25-33"},
			},
			want:     false,
			template: "custom-{name}-{number}",
			nextNum:  7,
		},
		{
			name: "template_equal_to_name_is_detected",
			series: &raidseries.Series{
				Name: "ICC25-01", NameTemplate: "ICC25-01", NextRaidNumber: 1,
			},
			raids: []raid.Raid{
				{Name: "ICC25-01"},
				{Name: "ICC25-33"},
			},
			want:     true,
			template: "ICC25-{number}",
			nextNum:  34,
		},
		{
			name: "sequential_numbering",
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc25-02"},
				{Name: "icc25-33"},
			},
			want:     true,
			template: "icc25-{number}",
			nextNum:  34,
		},
		{
			name: "single_raid",
			raids: []raid.Raid{
				{Name: "icc25-01"},
			},
			want:     true,
			template: "icc25-{number}",
			nextNum:  2,
		},
		{
			name: "inconsistent_prefix",
			raids: []raid.Raid{
				{Name: "icc25-01"},
				{Name: "icc10-02"},
			},
			want: false,
		},
		{
			name: "non_numeric_suffix",
			raids: []raid.Raid{
				{Name: "icc25-alpha"},
			},
			want: false,
		},
		{
			name: "no_dash_separator",
			raids: []raid.Raid{
				{Name: "icc2501"},
			},
			want: false,
		},
		{
			name: "zero_number",
			raids: []raid.Raid{
				{Name: "icc25-00"},
			},
			want: false,
		},
		{
			name: "underscore_delimiter_not_detected",
			raids: []raid.Raid{
				{Name: "icc25_01"},
				{Name: "icc25_02"},
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.series
			if s == nil {
				s = &raidseries.Series{}
			}
			got := detectPattern(s, tc.raids)
			if got != tc.want {
				t.Fatalf("want %v got %v", tc.want, got)
			}
			if tc.want || tc.series != nil {
				if s.NameTemplate != tc.template {
					t.Fatalf("want template %q got %q", tc.template, s.NameTemplate)
				}
				if s.NextRaidNumber != tc.nextNum {
					t.Fatalf("want next number %d got %d", tc.nextNum, s.NextRaidNumber)
				}
			}
		})
	}
}

func TestReqPlatform011_AC03(t *testing.T) {
	srv := newHTTPServer(http.NewServeMux())
	if srv.ReadHeaderTimeout != 10*time.Second || srv.ReadTimeout != 30*time.Second ||
		srv.WriteTimeout != 60*time.Second || srv.IdleTimeout != 120*time.Second {
		t.Fatalf("timeouts = %v/%v/%v/%v, want SC-026 10s/30s/60s/120s",
			srv.ReadHeaderTimeout, srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout)
	}
	if shutdownTimeout != 25*time.Second {
		t.Fatalf("shutdownTimeout = %v, want 25s", shutdownTimeout)
	}
}

func TestReqPlatform011_AC01(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newHTTPServer(http.NewServeMux())
	srv.ReadHeaderTimeout = 50 * time.Millisecond // shortened only to keep the test fast
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln, time.Second) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Incomplete header block: the server must close the connection.
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.Copy(io.Discard, conn); err != nil {
		t.Fatalf("connection was not closed by the server: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReqPlatform011_AC02(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	srv := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln, 10*time.Second) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	<-started
	cancel() // stands in for SIGTERM

	select {
	case err := <-done:
		t.Fatalf("serve returned while a request was running: %v", err)
	default:
	}
	close(release)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "done" {
		t.Fatalf("response = %d %q", resp.StatusCode, body)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve = %v, want nil", err)
	}
}
