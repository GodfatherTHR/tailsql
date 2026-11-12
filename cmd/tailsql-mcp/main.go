// Command tailsql-mcp bridges MCP JSON-RPC lines from stdio to the TailSQL
// HTTP MCP server. It lets agents that only speak stdio (for example a local
// Claude Desktop config) use the same tools without duplicating any logic.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:9099/mcp", "TailSQL MCP endpoint")
	token := flag.String("token", os.Getenv("TAILSQL_MCP_TOKEN"), "Bearer token (or TAILSQL_MCP_TOKEN)")
	flag.Parse()

	client := &http.Client{Timeout: 120 * time.Second}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	reader := bufio.NewReader(os.Stdin)
	_ = reader

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		request, err := http.NewRequest(http.MethodPost, *url, bytes.NewReader([]byte(line)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
			continue
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if strings.TrimSpace(*token) != "" {
			request.Header.Set("Authorization", "Bearer "+*token)
		}
		response, err := client.Do(request)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		_ = response.Body.Close()
		if readErr != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", readErr)
			continue
		}
		if response.StatusCode == http.StatusAccepted || len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		if _, err := writer.Write(body); err != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
			continue
		}
		if err := writer.WriteByte('\n'); err != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
			continue
		}
		if err := writer.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "tailsql-mcp:", err)
		os.Exit(1)
	}
}
