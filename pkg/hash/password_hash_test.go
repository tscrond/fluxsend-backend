package hash

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashPasswordArgon2idRejectsWhenWorkersAreBusy(t *testing.T) {
	for range cap(argon2WorkerSlots) {
		argon2WorkerSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range cap(argon2WorkerSlots) {
			<-argon2WorkerSlots
		}
	})

	_, err := HashPasswordArgon2id("password")
	require.ErrorIs(t, err, ErrArgon2Busy)
}
