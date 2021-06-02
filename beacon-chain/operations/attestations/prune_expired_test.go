package attestations

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/go-bitfield"
	ethpb "github.com/prysmaticlabs/prysm/proto/eth/v1alpha1"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/prysmaticlabs/prysm/shared/runutil"
	"github.com/prysmaticlabs/prysm/shared/testutil"
	"github.com/prysmaticlabs/prysm/shared/testutil/assert"
	"github.com/prysmaticlabs/prysm/shared/testutil/require"
	"github.com/prysmaticlabs/prysm/shared/timeutils"
)

func TestPruneExpired_Ticker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	s, err := NewService(ctx, &Config{
		Pool:          NewPool(),
		pruneInterval: 250 * time.Millisecond,
	})
	require.NoError(t, err)

	ad1 := testutil.HydrateAttestationData(&ethpb.AttestationData{})

	ad2 := testutil.HydrateAttestationData(&ethpb.AttestationData{Slot: 1})

	atts := []*ethpb.Attestation{
		{Data: ad1, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signature: make([]byte, 96)},
		{Data: ad2, AggregationBits: bitfield.Bitlist{0b1000, 0b1}, Signature: make([]byte, 96)},
	}
	require.NoError(t, s.cfg.Pool.SaveUnaggregatedAttestations(atts))
	require.Equal(t, 2, s.cfg.Pool.UnaggregatedAttestationCount(), "Unexpected number of attestations")
	atts = []*ethpb.Attestation{
		{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signature: make([]byte, 96)},
		{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101, 0b1}, Signature: make([]byte, 96)},
	}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	assert.Equal(t, 2, s.cfg.Pool.AggregatedAttestationCount())
	require.NoError(t, s.cfg.Pool.SaveBlockAttestations(atts))

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(timeutils.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))

	go s.pruneAttsPool()

	done := make(chan struct{}, 1)
	runutil.RunEvery(ctx, 500*time.Millisecond, func() {
		atts, err := s.cfg.Pool.UnaggregatedAttestations()
		require.NoError(t, err)
		for _, attestation := range atts {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		for _, attestation := range s.cfg.Pool.BlockAttestations() {
			if attestation.Data.Slot == 0 {
				return
			}
		}
		if s.cfg.Pool.UnaggregatedAttestationCount() != 1 || s.cfg.Pool.AggregatedAttestationCount() != 1 {
			return
		}
		done <- struct{}{}
	})
	select {
	case <-done:
		// All checks are passed.
	case <-ctx.Done():
		t.Error("Test case takes too long to complete")
	}
}

func TestPruneExpired_PruneExpiredAtts(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	ad1 := testutil.HydrateAttestationData(&ethpb.AttestationData{})

	ad2 := testutil.HydrateAttestationData(&ethpb.AttestationData{})

	att1 := &ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1101}}
	att2 := &ethpb.Attestation{Data: ad1, AggregationBits: bitfield.Bitlist{0b1111}}
	att3 := &ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1101}}
	att4 := &ethpb.Attestation{Data: ad2, AggregationBits: bitfield.Bitlist{0b1110}}
	atts := []*ethpb.Attestation{att1, att2, att3, att4}
	require.NoError(t, s.cfg.Pool.SaveAggregatedAttestations(atts))
	require.NoError(t, s.cfg.Pool.SaveBlockAttestations(atts))

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(timeutils.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))

	s.pruneExpiredAtts()
	// All the attestations on slot 0 should be pruned.
	for _, attestation := range s.cfg.Pool.AggregatedAttestations() {
		if attestation.Data.Slot == 0 {
			t.Error("Should be pruned")
		}
	}
	for _, attestation := range s.cfg.Pool.BlockAttestations() {
		if attestation.Data.Slot == 0 {
			t.Error("Should be pruned")
		}
	}
}

func TestPruneExpired_Expired(t *testing.T) {
	s, err := NewService(context.Background(), &Config{Pool: NewPool()})
	require.NoError(t, err)

	// Rewind back one epoch worth of time.
	s.genesisTime = uint64(timeutils.Now().Unix()) - uint64(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	assert.Equal(t, true, s.expired(0), "Should be expired")
	assert.Equal(t, false, s.expired(1), "Should not be expired")
}
