package p2p

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	envP2PSecret   = "PTERMINAL_P2P_SECRET"
	envP2PInsecure = "PTERMINAL_P2P_INSECURE"

	helloAuthWindow = 45 * time.Second
	maxFrameBytes   = 64 * 1024 * 1024

	nonceSize = 12
)

type codec interface {
	Encode(v any) error
	Decode(v any) error
}

type jsonCodec struct {
	enc *json.Encoder
	dec *json.Decoder
}

func (c *jsonCodec) Encode(v any) error { return c.enc.Encode(v) }
func (c *jsonCodec) Decode(v any) error { return c.dec.Decode(v) }

type secureCodec struct {
	reader *bufio.Reader
	writer io.Writer
	gcm    cipher.AEAD
	mu     sync.Mutex
}

func (c *secureCodec) Encode(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := c.gcm.Seal(nil, nonce, payload, nil)
	frame := append(nonce, ciphertext...)
	if len(frame) > maxFrameBytes {
		return fmt.Errorf("p2p frame too large: %d bytes", len(frame))
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(frame)))

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.writer.Write(hdr[:]); err != nil {
		return err
	}
	_, err = c.writer.Write(frame)
	return err
}

func (c *secureCodec) Decode(v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(c.reader, hdr[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(hdr[:])
	if size == 0 || size > maxFrameBytes {
		return fmt.Errorf("invalid p2p frame size: %d", size)
	}

	frame := make([]byte, size)
	if _, err := io.ReadFull(c.reader, frame); err != nil {
		return err
	}
	if len(frame) <= nonceSize {
		return errors.New("invalid p2p frame")
	}

	nonce := frame[:nonceSize]
	ciphertext := frame[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	return json.Unmarshal(plaintext, v)
}

func newCodec(conn io.ReadWriter, secret []byte) (codec, error) {
	if len(secret) == 0 {
		br := bufio.NewReader(conn)
		return &jsonCodec{
			enc: json.NewEncoder(conn),
			dec: json.NewDecoder(br),
		}, nil
	}

	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &secureCodec{
		reader: bufio.NewReader(conn),
		writer: conn,
		gcm:    gcm,
	}, nil
}

func loadSecret() (secret []byte, insecure bool, err error) {
	raw := strings.TrimSpace(os.Getenv(envP2PSecret))
	if raw == "" {
		if os.Getenv(envP2PInsecure) == "1" {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("%s is not set", envP2PSecret)
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], false, nil
}

func helloAuth(secret []byte, deviceID string, tcpPort int, ts int64) string {
	if len(secret) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%d|%d", deviceID, tcpPort, ts)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyHello(secret []byte, deviceID string, tcpPort int, ts int64, auth string, now time.Time) bool {
	if len(secret) == 0 {
		return true
	}
	if auth == "" || ts == 0 {
		return false
	}
	delta := now.Sub(time.Unix(ts, 0))
	if delta < -helloAuthWindow || delta > helloAuthWindow {
		return false
	}
	expected := helloAuth(secret, deviceID, tcpPort, ts)
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	incomingBytes, err := hex.DecodeString(auth)
	if err != nil {
		return false
	}
	return hmac.Equal(expectedBytes, incomingBytes)
}
