package main

import (
        "io"
        "log"
        "net/http"
        "net/url"
        "strings"
        "time"

        "github.com/aws/aws-sdk-go/aws/credentials"
        "github.com/aws/aws-sdk-go/aws/signer/v4"
)

func main() {
        // Use the base S3 endpoint
        target := "https://w4h8.fra.idrivee2-22.com"
        targetURL, _ := url.Parse(target)

        creds := credentials.NewStaticCredentials(
                "<key>",
                "<secret key>",
                "",
        )

        signer := v4.NewSigner(creds)

        http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
                log.Printf("Received request: %s %s", r.Method, r.URL.Path)

                // Allow only GET and HEAD requests (read-only)
                if r.Method != "GET" && r.Method != "HEAD" {
                        log.Printf("Blocked non-read request: %s", r.Method)
                        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
                        return
                }

                // The path should be the original path without modification
                // since the bucket name is already in the host
                path := r.URL.Path
                if path == "" {
                        path = "/"
                }

                // Create a new request to S3
                s3URL := target + path
                log.Printf("Proxying to S3: %s", s3URL)

                proxyReq, err := http.NewRequest(r.Method, s3URL, r.Body)
                if err != nil {
                        log.Printf("Error creating request: %v", err)
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }

                // Set the Host header to include the bucket name
                proxyReq.Host = "mastodon." + targetURL.Host

                // Remove headers that might interfere with the signature
                proxyReq.Header = make(http.Header)

                // Copy only specific headers from the original request
                for name, values := range r.Header {
                        switch name {
                        case "Range", "If-Modified-Since", "If-None-Match", "Cache-Control", "Content-Type":
                                for _, value := range values {
                                        proxyReq.Header.Add(name, value)
                                }
                        }
                }

                // Sign the request with AWS Signature V4
                _, err = signer.Sign(proxyReq, nil, "s3", "us-east-1", time.Now())
                if err != nil {
                        log.Printf("Error signing request: %v", err)
                        http.Error(w, err.Error(), http.StatusInternalServerError)
                        return
                }

                // Make the request to S3
                client := &http.Client{}
                resp, err := client.Do(proxyReq)
                if err != nil {
                        log.Printf("Error making request to S3: %v", err)
                        http.Error(w, err.Error(), http.StatusBadGateway)
                        return
                }
                defer resp.Body.Close()

                log.Printf("S3 response status: %s", resp.Status)

                // Copy response headers
                for name, values := range resp.Header {
                        // Skip unwanted headers
                        if name == "Set-Cookie" ||
                           strings.HasPrefix(name, "X-Amz-") ||
                           name == "Connection" {
                                continue
                        }
                        for _, value := range values {
                                w.Header().Add(name, value)
                        }
                }

                // Add CORS headers
                w.Header().Set("Access-Control-Allow-Origin", "*")
                w.Header().Set("X-Content-Type-Options", "nosniff")
                w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'none'")
                w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

                // Copy status code
                w.WriteHeader(resp.StatusCode)

                // Copy response body (only for GET requests, not HEAD)
                if r.Method == "GET" {
                        _, err = io.Copy(w, resp.Body)
                        if err != nil {
                                log.Printf("Error copying response body: %v", err)
                        }
                }
        })

        log.Println("Starting S3 signing proxy on :8080")
        log.Fatal(http.ListenAndServe(":8080", nil))
}