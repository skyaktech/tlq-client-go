package tlq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultHost       = "localhost"
	DefaultPort       = 1337
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3
	DefaultRetryDelay = 100 * time.Millisecond
	MaxMessageSize    = 64 * 1024 // 64KB
)

type Config struct {
	Host       string
	Port       int
	Timeout    time.Duration
	MaxRetries int
	RetryDelay time.Duration
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	config     *Config
}

type Message struct {
	ID         string `json:"id"`
	Body       string `json:"body"`
	State      string `json:"state"`
	RetryCount int    `json:"retry_count"`
}

type Option func(*Config)

func WithHost(host string) Option {
	return func(c *Config) {
		c.Host = host
	}
}

func WithPort(port int) Option {
	return func(c *Config) {
		c.Port = port
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

func WithMaxRetries(maxRetries int) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
	}
}

func WithRetryDelay(delay time.Duration) Option {
	return func(c *Config) {
		c.RetryDelay = delay
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

func NewClient(opts ...Option) *Client {
	config := &Config{
		Host:       DefaultHost,
		Port:       DefaultPort,
		Timeout:    DefaultTimeout,
		MaxRetries: DefaultMaxRetries,
		RetryDelay: DefaultRetryDelay,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	return &Client{
		baseURL:    fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		httpClient: config.HTTPClient,
		config:     config,
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + path

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay * time.Duration(attempt)):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/hello", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) AddMessage(ctx context.Context, body string) (*Message, error) {
	if len(body) > MaxMessageSize {
		return nil, fmt.Errorf("message body exceeds maximum size of %d bytes", MaxMessageSize)
	}

	payload := map[string]string{"body": body}
	resp, err := c.doRequest(ctx, http.MethodPost, "/add", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var message Message
	if err := json.NewDecoder(resp.Body).Decode(&message); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &message, nil
}

func (c *Client) GetMessages(ctx context.Context, count int) ([]*Message, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be greater than 0")
	}

	payload := map[string]int{"count": count}
	resp, err := c.doRequest(ctx, http.MethodPost, "/get", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var messages []*Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return messages, nil
}

func (c *Client) DeleteMessages(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return fmt.Errorf("no message IDs provided")
	}

	payload := map[string][]string{"ids": messageIDs}
	resp, err := c.doRequest(ctx, http.MethodPost, "/delete", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	return c.DeleteMessages(ctx, []string{messageID})
}

func (c *Client) RetryMessages(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return fmt.Errorf("no message IDs provided")
	}

	payload := map[string][]string{"ids": messageIDs}
	resp, err := c.doRequest(ctx, http.MethodPost, "/retry", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) RetryMessage(ctx context.Context, messageID string) error {
	return c.RetryMessages(ctx, []string{messageID})
}

func (c *Client) PurgeQueue(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/purge", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
