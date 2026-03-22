// Package caster implements the NTRIP TCP server and protocol handling.
package caster

import (
	"bufio"
	"fmt"
	"strings"
)

// RequestType identifies the kind of NTRIP request.
type RequestType int

const (
	RequestUnknown    RequestType = iota
	RequestSourcetable            // GET /
	RequestRover                  // GET /mountpoint
	RequestSourceRev1             // SOURCE password /mountpoint
	RequestSourceRev2             // POST /mountpoint HTTP/1.1
)

// NTRIPRequest holds the parsed first line and headers of an incoming NTRIP request.
type NTRIPRequest struct {
	Type       RequestType
	MountPoint string // without leading /
	Password   string // Rev1 SOURCE password
	Headers    map[string]string
	Proto      string // "HTTP/1.0" or "HTTP/1.1"
}

// ParseRequest reads the request line and headers from a buffered reader.
func ParseRequest(r *bufio.Reader) (*NTRIPRequest, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read request line: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")

	req := &NTRIPRequest{
		Headers: make(map[string]string),
	}

	parts := strings.Fields(line)
	if len(parts) < 1 {
		return nil, fmt.Errorf("empty request line")
	}

	switch parts[0] {
	case "GET":
		if len(parts) < 2 {
			return nil, fmt.Errorf("malformed GET request")
		}
		path := parts[0+1]
		if len(parts) >= 3 {
			req.Proto = parts[2]
		}
		if err := readHeaders(r, req); err != nil {
			return nil, err
		}
		if path == "/" {
			req.Type = RequestSourcetable
		} else {
			req.Type = RequestRover
			req.MountPoint = strings.TrimPrefix(path, "/")
		}

	case "SOURCE":
		// SOURCE password /mountpoint
		if len(parts) < 3 {
			return nil, fmt.Errorf("malformed SOURCE request")
		}
		req.Type = RequestSourceRev1
		req.Password = parts[1]
		req.MountPoint = strings.TrimPrefix(parts[2], "/")
		if err := readHeaders(r, req); err != nil {
			return nil, err
		}

	case "POST":
		if len(parts) < 2 {
			return nil, fmt.Errorf("malformed POST request")
		}
		req.Type = RequestSourceRev2
		req.MountPoint = strings.TrimPrefix(parts[1], "/")
		if len(parts) >= 3 {
			req.Proto = parts[2]
		}
		if err := readHeaders(r, req); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported method: %s", parts[0])
	}

	return req, nil
}

func readHeaders(r *bufio.Reader, req *NTRIPRequest) error {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		req.Headers[key] = val
	}
	return nil
}
