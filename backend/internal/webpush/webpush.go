// Package webpush delivers the daily briefing over the Web Push protocol
// (replaces the M2 Expo transport). VAPID keys are generated on first boot and
// persisted in app_settings; expired subscriptions are pruned on send.
package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	wp "github.com/SherClockHolmes/webpush-go"

	"help-my-run/backend/internal/store"
)

// subscriberContact is the VAPID "sub" claim (a contact for push services).
const subscriberContact = "https://github.com/jlee0425/help-my-run"

// Service sends Web Push notifications to every stored subscription.
// HTTPClient is injectable for tests (nil -> http.DefaultClient).
type Service struct {
	Store      *store.Store
	HTTPClient *http.Client

	vapidPublic  string
	vapidPrivate string
}

// payload is the JSON the service worker's push handler expects.
type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// New loads the VAPID keypair from app_settings, generating and persisting a
// fresh pair on first boot.
func New(s *store.Store) (*Service, error) {
	pub, pubErr := s.GetSetting(store.SettingVAPIDPublic)
	priv, privErr := s.GetSetting(store.SettingVAPIDPrivate)
	if errors.Is(pubErr, store.ErrNotFound) || errors.Is(privErr, store.ErrNotFound) {
		var err error
		priv, pub, err = wp.GenerateVAPIDKeys()
		if err != nil {
			return nil, fmt.Errorf("webpush: generate VAPID keys: %w", err)
		}
		if err := s.SetSetting(store.SettingVAPIDPublic, pub); err != nil {
			return nil, err
		}
		if err := s.SetSetting(store.SettingVAPIDPrivate, priv); err != nil {
			return nil, err
		}
	} else if pubErr != nil {
		return nil, pubErr
	} else if privErr != nil {
		return nil, privErr
	}
	return &Service{Store: s, vapidPublic: pub, vapidPrivate: priv}, nil
}

// PublicKey returns the VAPID public key (base64url) for pushManager.subscribe.
func (w *Service) PublicKey() string { return w.vapidPublic }

// Send implements the agent's Pusher seam: broadcast to every subscription.
func (w *Service) Send(ctx context.Context, title, body, url string) error {
	return w.Broadcast(ctx, title, body, url)
}

// Broadcast pushes to all subscriptions. 404/410 responses prune the
// subscription. Returns nil when at least one delivery succeeded.
func (w *Service) Broadcast(ctx context.Context, title, body, url string) error {
	subs, err := w.Store.ListPushSubscriptions()
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return errors.New("webpush: no subscriptions registered")
	}
	msg, err := json.Marshal(payload{Title: title, Body: body, URL: url})
	if err != nil {
		return err
	}

	delivered := false
	var lastErr error
	for _, sub := range subs {
		s := &wp.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     wp.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}
		resp, serr := wp.SendNotificationWithContext(ctx, msg, s, &wp.Options{
			Subscriber:      subscriberContact,
			VAPIDPublicKey:  w.vapidPublic,
			VAPIDPrivateKey: w.vapidPrivate,
			TTL:             12 * 60 * 60, // briefing is stale after ~12h
			HTTPClient:      w.HTTPClient,
		})
		if serr != nil {
			lastErr = serr
			log.Printf("webpush: send to %.40s… failed: %v", sub.Endpoint, serr)
			continue
		}
		func() {
			defer resp.Body.Close()
			switch {
			case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
				// Browser dropped the subscription — prune it.
				_ = w.Store.DeletePushSubscription(sub.Endpoint)
				lastErr = fmt.Errorf("webpush: subscription gone (%d)", resp.StatusCode)
			case resp.StatusCode >= 400:
				lastErr = fmt.Errorf("webpush: push service returned %d", resp.StatusCode)
				log.Printf("webpush: send to %.40s… returned %d", sub.Endpoint, resp.StatusCode)
			default:
				delivered = true
			}
		}()
	}
	if delivered {
		return nil
	}
	return lastErr
}
