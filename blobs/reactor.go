package blobs

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/p2p"
	protoblobs "github.com/cometbft/cometbft/proto/tendermint/blobs"
)

// Reactor handles blob broadcasting amongst peers.
type Reactor struct {
	p2p.BaseReactor

	dataSizeBytes int
	myTurn        bool
}

// NewReactor returns a new Reactor.
func NewReactor(config *config.BlobsConfig) *Reactor {
	memR := &Reactor{
		myTurn:        config.SendFirst,
		dataSizeBytes: config.DataSizeBytes,
	}
	memR.BaseReactor = *p2p.NewBaseReactor("Blobs", memR)

	return memR
}

// GetChannels implements Reactor by returning the list of channels for this
// reactor.
func (blobsR *Reactor) GetChannels() []*p2p.ChannelDescriptor {
	return []*p2p.ChannelDescriptor{
		{
			ID:                  BlobsChannel,
			Priority:            5,
			RecvMessageCapacity: 1e9, // 1 GB
			MessageType:         &protoblobs.Message{},
		},
	}
}

// Receive implements Reactor.
// It acknowledges any received blobs.
func (blobsR *Reactor) Receive(e p2p.Envelope) {
	blobsR.Logger.Debug("Receive", "src", e.Src, "chId", e.ChannelID, "msg", e.Message)
	switch msg := e.Message.(type) {
	case *protoblobs.Blob:
		blobData := msg.GetData()
		if len(blobData) == 0 {
			blobsR.Logger.Error("received empty blob from peer", "src", e.Src)
			return
		}

		blobsR.Logger.Info("received blob of size %d", len(blobData))
		blobsR.myTurn = true

	default:
		blobsR.Logger.Error("unknown message type", "src", e.Src, "chId", e.ChannelID, "msg", e.Message)
		blobsR.Switch.StopPeerForError(e.Src, fmt.Errorf("blobs cannot handle message of type: %T", e.Message))
		return
	}

	// broadcasting happens from go routines per peer
}

func generateRandomData(size int) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)
	return data
}

// Send new blobs to peer.
func (blobsR *Reactor) broadcastBlobRoutine(peer p2p.Peer) {

	for {
		if !blobsR.IsRunning() || !peer.IsRunning() {
			return
		}

		if blobsR.myTurn {
			blobsR.Logger.Info("my turn...")
			data := generateRandomData(blobsR.dataSizeBytes)
			blobsR.Logger.Info("generated data...")
			success := peer.Send(p2p.Envelope{
				ChannelID: BlobsChannel,
				Message:   &protoblobs.Blob{Data: data},
			})
			if !success {
				time.Sleep(UnsuccessfulSendSleepIntervalMS * time.Millisecond)
				continue
			}

			blobsR.myTurn = false
		}

		select {
		case <-time.After(SuccessfulSendSleepIntervalMS * time.Millisecond):
			blobsR.Logger.Info("sleeping...")
			continue
		case <-peer.Quit():
			return
		case <-blobsR.Quit():
			return
		}
	}
}
