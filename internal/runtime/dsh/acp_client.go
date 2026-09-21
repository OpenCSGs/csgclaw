package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ACP error %d: %s", e.Code, e.Message)
}

type rpcReply struct {
	result json.RawMessage
	err    error
}

type serverRequest struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type notification struct {
	Method string
	Params json.RawMessage
}

type acpClient struct {
	reader io.Reader
	writer io.Writer

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcReply
	closed  chan struct{}
	err     error

	handlerMu      sync.RWMutex
	onRequest      func(serverRequest)
	onNotification func(notification)
}

func (c *acpClient) setHandlers(onRequest func(serverRequest), onNotification func(notification)) {
	c.handlerMu.Lock()
	c.onRequest = onRequest
	c.onNotification = onNotification
	c.handlerMu.Unlock()
}

func newACPClient(reader io.Reader, writer io.Writer) *acpClient {
	client := &acpClient{
		reader:  reader,
		writer:  writer,
		pending: map[int64]chan rpcReply{},
		closed:  make(chan struct{}),
	}
	go client.readLoop()
	return client
}

func (c *acpClient) call(ctx context.Context, method string, params any, result any, accepted func()) error {
	id, replies, err := c.sendRequest(method, params)
	if err != nil {
		return err
	}
	if accepted != nil {
		accepted()
	}
	select {
	case reply := <-replies:
		return decodeACPReply(method, result, reply)
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.closed:
		return c.closeError()
	}
}

var errACPCancellationTimeout = errors.New("timed out waiting for ACP request cancellation")

// callRetainingOnCancel keeps the request registered after ctx is canceled so
// the caller can hold its conversation slot until the ACP peer sends the
// terminal response. This prevents late updates from a canceled prompt from
// being attributed to a replacement turn using the same session.
func (c *acpClient) callRetainingOnCancel(
	ctx context.Context,
	method string,
	params any,
	result any,
	accepted func(),
	cancelWait time.Duration,
	onCancel func() error,
) error {
	id, replies, err := c.sendRequest(method, params)
	if err != nil {
		return err
	}
	if accepted != nil {
		accepted()
	}
	select {
	case reply := <-replies:
		return decodeACPReply(method, result, reply)
	case <-ctx.Done():
		var cancelErr error
		if onCancel != nil {
			cancelErr = onCancel()
		}
		if cancelWait <= 0 {
			cancelWait = time.Second
		}
		timer := time.NewTimer(cancelWait)
		defer timer.Stop()
		select {
		case <-replies:
			return errors.Join(ctx.Err(), cancelErr)
		case <-c.closed:
			return errors.Join(ctx.Err(), cancelErr)
		case <-timer.C:
			c.removePending(id)
			return errors.Join(ctx.Err(), cancelErr, errACPCancellationTimeout)
		}
	case <-c.closed:
		return c.closeError()
	}
}

func decodeACPReply(method string, result any, reply rpcReply) error {
	if reply.err != nil {
		return reply.err
	}
	if result == nil || len(reply.result) == 0 || string(reply.result) == "null" {
		return nil
	}
	if err := json.Unmarshal(reply.result, result); err != nil {
		return fmt.Errorf("decode ACP %s response: %w", method, err)
	}
	return nil
}

func (c *acpClient) notify(method string, params any) error {
	return c.writeFrame(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *acpClient) respond(id json.RawMessage, result any, responseErr *rpcError) error {
	frame := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id)}
	if responseErr != nil {
		frame["error"] = responseErr
	} else {
		frame["result"] = result
	}
	return c.writeFrame(frame)
}

func (c *acpClient) sendRequest(method string, params any) (int64, <-chan rpcReply, error) {
	c.mu.Lock()
	select {
	case <-c.closed:
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = io.EOF
		}
		return 0, nil, err
	default:
	}
	c.nextID++
	id := c.nextID
	replies := make(chan rpcReply, 1)
	c.pending[id] = replies
	c.mu.Unlock()
	if err := c.writeFrame(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.removePending(id)
		return 0, nil, err
	}
	return id, replies, nil
}

func (c *acpClient) writeFrame(frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.writer.Write(data)
	return err
}

func (c *acpClient) readLoop() {
	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var envelope struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			c.fail(fmt.Errorf("decode ACP frame: %w", err))
			return
		}
		if envelope.Method != "" {
			if len(envelope.ID) > 0 {
				c.handlerMu.RLock()
				onRequest := c.onRequest
				c.handlerMu.RUnlock()
				if onRequest != nil {
					go onRequest(serverRequest{ID: envelope.ID, Method: envelope.Method, Params: envelope.Params})
				}
			} else {
				c.handlerMu.RLock()
				onNotification := c.onNotification
				c.handlerMu.RUnlock()
				if onNotification != nil {
					onNotification(notification{Method: envelope.Method, Params: envelope.Params})
				}
			}
			continue
		}
		var id int64
		if err := json.Unmarshal(envelope.ID, &id); err != nil {
			continue
		}
		c.mu.Lock()
		replies := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if replies != nil {
			if envelope.Error != nil {
				replies <- rpcReply{err: envelope.Error}
			} else {
				replies <- rpcReply{result: envelope.Result}
			}
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	c.fail(err)
}

func (c *acpClient) fail(err error) {
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return
	default:
	}
	c.err = err
	close(c.closed)
	for id, replies := range c.pending {
		replies <- rpcReply{err: err}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func (c *acpClient) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *acpClient) closeError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return errors.New("ACP connection closed")
	}
	return c.err
}
