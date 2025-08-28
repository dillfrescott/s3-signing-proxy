package main

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/signer/v4"
)

func main() {
	target := "https://w4h8.fra.idrivee2-22.com"
	targetURL, _ := url.Parse(target)

	creds := credentials.NewStaticCredentials(
		"<key>",
		"<secret key>",
		"",
	)

	signer := v4.NewSigner(creds)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("S3 Proxy"))
			return
		}

		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Path
		if path == "" {
			path = "/"
		}

		s3URL := target + path

		proxyReq, err := http.NewRequest(r.Method, s3URL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		proxyReq.Host = "mastodon." + targetURL.Host
		proxyReq.Header = make(http.Header)

		for name, values := range r.Header {
			switch name {
			case "Range", "If-Modified-Since", "If-None-Match", "Cache-Control", "Content-Type":
				for _, value := range values {
					proxyReq.Header.Add(name, value)
				}
			}
		}

		_, err = signer.Sign(proxyReq, nil, "s3", "", time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for name, values := range resp.Header {
			if name == "Set-Cookie" ||
				strings.HasPrefix(name, "X-Amz-") ||
				name == "Connection" {
				continue
			}
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'none'")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		w.WriteHeader(resp.StatusCode)

		if r.Method == "GET" {
			_, _ = io.Copy(w, resp.Body)
		}
	})

	http.ListenAndServe(":8080", nil)
}

