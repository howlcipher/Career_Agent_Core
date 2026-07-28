package main

import (
	"fmt"
	"net/url"
	"strings"
)

func main() {
	u, _ := url.Parse("https://boards.greenhouse.io/remotecom/jobs/123")
	parts := strings.Split(u.Path, "/")
	if len(parts) >= 2 {
		fmt.Printf("Prefix: %s://%s/%s\n", u.Scheme, u.Host, parts[1])
	}
}
