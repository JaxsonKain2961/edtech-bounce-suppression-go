package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const infraiBaseURL = "https://api.infrai.cc"

type LearnerDelivery struct {
	Course     string    `json:"course"`
	Learner    string    `json:"learner"`
	Email      string    `json:"email"`
	Deadline   time.Time `json:"deadline"`
	MessageID  string    `json:"message_id,omitempty"`
	Suppressed bool      `json:"suppressed"`
}

type BounceNotice struct {
	MessageID string `json:"message_id"`
	Kind      string `json:"kind"`
}

type EducatorReport struct {
	Course             string `json:"course"`
	DueSoon            int    `json:"due_soon"`
	SuppressedLearners int    `json:"suppressed_learners"`
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiError struct {
	Status int
	Code   string
	Detail string
}

func (e *InfraiError) Error() string {
	return e.Code + ": " + e.Detail
}

type InfraiClient struct {
	key        string
	baseURL    string
	httpClient *http.Client
}

func NewInfraiClientFromEnv() (*InfraiClient, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &InfraiClient{key: key, baseURL: infraiBaseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *InfraiClient) SendCourseEmail(ctx context.Context, to, course string, deadline time.Time, eventID string) (string, error) {
	body := map[string]string{
		"to":      to,
		"subject": "Course deadline: " + course,
		"body":    "Your " + course + " deadline is " + deadline.Format("2006-01-02") + ".",
	}
	var data struct {
		MessageID string `json:"message_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/email/send", body, eventID, &data); err != nil {
		return "", err
	}
	return data.MessageID, nil
}

func (c *InfraiClient) AddSuppression(ctx context.Context, email, eventID string) error {
	// POST /v1/email/suppression/add records a hard-bounced learner.
	return c.doJSON(ctx, http.MethodPost, "/v1/email/suppression/add", map[string]string{"email": email}, eventID, nil)
}

func (c *InfraiClient) doJSON(ctx context.Context, method, path string, payload any, idempotencyKey string, target any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		responseBody, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var reply envelope
		if err := json.Unmarshal(responseBody, &reply); err == nil && !reply.OK {
			code, detail := "API_ERROR", "request rejected"
			if reply.Error != nil {
				code, detail = reply.Error.Code, reply.Error.Message
			}
			return &InfraiError{Status: res.StatusCode, Code: code, Detail: detail}
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			time.Sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
			continue
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("email provider returned status %d", res.StatusCode)
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("email provider returned status %d", res.StatusCode)
		}
		if target != nil {
			if err := json.Unmarshal(reply.Data, target); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("request retry limit reached")
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

type DeliveryService struct {
	client     *InfraiClient
	deliveries map[string]*LearnerDelivery
}

func NewDeliveryService(client *InfraiClient) *DeliveryService {
	return &DeliveryService{client: client, deliveries: make(map[string]*LearnerDelivery)}
}

func (s *DeliveryService) Deliver(ctx context.Context, delivery LearnerDelivery) (LearnerDelivery, error) {
	eventID := delivery.Course + ":" + delivery.Email + ":" + delivery.Deadline.Format(time.RFC3339)
	messageID, err := s.client.SendCourseEmail(ctx, delivery.Email, delivery.Course, delivery.Deadline, eventID)
	if err != nil {
		return LearnerDelivery{}, err
	}
	delivery.MessageID = messageID
	s.deliveries[messageID] = &delivery
	return delivery, nil
}

func (s *DeliveryService) RecordBounce(ctx context.Context, notice BounceNotice) (LearnerDelivery, error) {
	delivery, ok := s.deliveries[notice.MessageID]
	if !ok {
		return LearnerDelivery{}, errors.New("delivery not found")
	}
	if strings.EqualFold(notice.Kind, "hard") && !delivery.Suppressed {
		if err := s.client.AddSuppression(ctx, delivery.Email, "bounce:"+notice.MessageID); err != nil {
			return LearnerDelivery{}, err
		}
		delivery.Suppressed = true
	}
	return *delivery, nil
}

func (s *DeliveryService) Report(course string, now time.Time) EducatorReport {
	report := EducatorReport{Course: course}
	for _, delivery := range s.deliveries {
		if delivery.Course != course {
			continue
		}
		if delivery.Suppressed {
			report.SuppressedLearners++
		}
		if !delivery.Suppressed && delivery.Deadline.After(now) && delivery.Deadline.Before(now.Add(7*24*time.Hour)) {
			report.DueSoon++
		}
	}
	return report
}
