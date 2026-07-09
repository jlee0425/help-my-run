package store

import "time"

// PushSubscription is one push_subscriptions row (a browser's Web Push
// subscription: endpoint URL + its two client keys).
type PushSubscription struct {
	Endpoint  string // PK
	P256dh    string
	Auth      string
	CreatedAt string
}

// UpsertPushSubscription inserts/refreshes a subscription by endpoint.
func (s *Store) UpsertPushSubscription(p PushSubscription) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO push_subscriptions (endpoint, p256dh, auth, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET
			p256dh = excluded.p256dh,
			auth   = excluded.auth`,
		p.Endpoint, p.P256dh, p.Auth, now)
	return err
}

// ListPushSubscriptions returns all subscriptions (most-recent-first).
func (s *Store) ListPushSubscriptions() ([]PushSubscription, error) {
	rows, err := s.DB.Query(`
		SELECT endpoint, p256dh, auth, created_at
		FROM push_subscriptions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PushSubscription, 0)
	for rows.Next() {
		var p PushSubscription
		if err := rows.Scan(&p.Endpoint, &p.P256dh, &p.Auth, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePushSubscription removes a subscription (unsubscribe or 404/410 prune).
// Missing endpoint is a no-op.
func (s *Store) DeletePushSubscription(endpoint string) error {
	_, err := s.DB.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}
