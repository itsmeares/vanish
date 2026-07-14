package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/itsmeares/vanish/internal/domain"
)

const actionIdempotencyKeyPrefix = "vanish-action-v1-"

// ActionIdempotencyKey is an opaque runtime-owned identity for one logical
// action within one durable execution. Providers must reuse it for every retry
// of that action and must not derive it from mutable provider state.
type ActionIdempotencyKey string

// ActionRequest carries the immutable cleanup action together with its stable
// runtime identity. The key is deliberately absent from cleanup-plan JSON.
type ActionRequest struct {
	Action         domain.CleanupAction
	IdempotencyKey ActionIdempotencyKey
}

func actionIdempotencyKey(executionID ExecutionID, actionID string) ActionIdempotencyKey {
	hash := sha256.New()
	writeIdentityPart(hash, "vanish-action-v1")
	writeIdentityPart(hash, string(executionID))
	writeIdentityPart(hash, actionID)
	return ActionIdempotencyKey(actionIdempotencyKeyPrefix + hex.EncodeToString(hash.Sum(nil)))
}

func writeIdentityPart(hash interface{ Write([]byte) (int, error) }, value string) {
	_, _ = hash.Write([]byte(strconv.Itoa(len(value))))
	_, _ = hash.Write([]byte{':'})
	_, _ = hash.Write([]byte(value))
}

func (key ActionIdempotencyKey) valid() bool {
	value := string(key)
	if len(value) != len(actionIdempotencyKeyPrefix)+sha256.Size*2 || value[:len(actionIdempotencyKeyPrefix)] != actionIdempotencyKeyPrefix {
		return false
	}
	encoded := value[len(actionIdempotencyKeyPrefix):]
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == encoded
}
