package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidCursor = errors.New("invalid or filter-mismatched cursor")

type opaqueCursor struct {
	Version int             `json:"v"`
	Kind    string          `json:"k"`
	Filter  string          `json:"f"`
	Data    json.RawMessage `json:"d"`
}

func filterDigest(kind string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode cursor filters: %w", err)
	}
	hash := sha256.Sum256(append([]byte(kind+":"), encoded...))
	return hex.EncodeToString(hash[:]), nil
}

func encodeOpaqueCursor(kind, filter string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode cursor position: %w", err)
	}
	encoded, err := json.Marshal(opaqueCursor{Version: 1, Kind: kind, Filter: filter, Data: data})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeOpaqueCursor(value, kind, filter string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return ErrInvalidCursor
	}
	var cursor opaqueCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || cursor.Kind != kind || cursor.Filter != filter || len(cursor.Data) == 0 {
		return ErrInvalidCursor
	}
	if err := json.Unmarshal(cursor.Data, target); err != nil {
		return ErrInvalidCursor
	}
	return nil
}

func NewUUID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func newUUID() string {
	return NewUUID()
}

