package redissignal

import (
	"encoding/json"
	"time"

	"github.com/go-redis/redis"
	log "github.com/pion/ion-log"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/pion/webrtc/v3"
)

var (
	SESSION_TIMEOUT = 10 * time.Second
)

// Signal represent the signals
type RedisSignalServer interface {
	RedisClient() *redis.Client
	ID() string

	SFUBus()
	GetSessionNode(sid string) (string, error)
	AttemptSessionLock(sid string) (string, error)
	RefreshSessionExpiry(sid string)
}

type redisSignalServer struct {
	ion    sfu.SFU
	client *redis.Client
	nodeID string
}

// NewRedisSignal will create an object that represent the Signal interface
func NewRedisSignalServer(ion sfu.SFU, client *redis.Client, nodeID string) RedisSignalServer {
	return &redisSignalServer{ion, client, nodeID}
}

func (s *redisSignalServer) RedisClient() *redis.Client {
	return s.client
}

func (s *redisSignalServer) ID() string {
	return s.nodeID
}

// SFUBus is the redis topic `sfu/` (for messages to all SFU, join methods)
func (s *redisSignalServer) SFUBus() {
	r := s.client
	topic := "sfu/" + s.nodeID
	log.Infof("SFUBus listening on 'sfu/' and '%s'", topic)

	for {

		message, err := r.BRPop(0, topic, "sfu/").Result()

		if err != nil {
			log.Errorf("sfu-bus: unrecognized %s", message)
			continue
		}

		var join Join

		err = json.Unmarshal([]byte(message[1]), &join)
		if err != nil {
			log.Errorf("sfu-bus: unrecognized %s", message)
			continue
		}

		locked_by, err := s.AttemptSessionLock(join.Sid)

		if err != nil {
			log.Errorf("error aquiring session lock %s", err)
		}
		if locked_by != s.nodeID {
			log.Infof("another node has session %s, forwarding join to sfu/%s", join.Sid, locked_by)
			r.LPush("sfu/"+locked_by, message[1])

			continue // another node aquired the session lock
		}

		log.Infof("joining room %s", join.Sid)

		p := sfu.NewPeer(&s.ion)

		sig := NewRedisSignal(s, join.Pid, join.Sid)

		p.OnOffer = func(offer *webrtc.SessionDescription) {
			message, _ := json.Marshal(Notify{"offer", offer, "2.0"})
			sig.SendToPeer(message)
		}

		p.OnIceCandidate = func(candidate *webrtc.ICECandidateInit) {
			message, _ := json.Marshal(Notify{"trickle", candidate, "2.0"})
			sig.SendToPeer(message)
		}

		answer, err := p.Join(join.Sid, join.Offer)

		if err != nil {
			log.Errorf("error joining %s %s", err)
		} else {
			log.Infof("peer %s joined session %s", join.Pid, join.Sid)
		}

		reply, err := json.Marshal(answer)

		// peer-recv/{id} channel is for peer to recieve messages
		sig.SendToPeer(reply)

		go sig.SFUPeerBus(&p)

	}
}

// SessionExists tells you if any other node has the session key locked
func (s *redisSignalServer) GetSessionNode(sid string) (string, error) {
	r := s.client
	result, err := r.Get("session/" + sid).Result()
	return result, err
}

// AttemptSessionLock returns true if no other node has a session lock, and locks the session
func (s *redisSignalServer) AttemptSessionLock(sid string) (string, error) {
	r := s.client

	sessionNode, err := s.GetSessionNode(sid)
	if sessionNode == "" {
		set, err := r.SetNX("session/"+sid, s.ID(), SESSION_TIMEOUT).Result()

		if err != nil {
			log.Errorf("error locking session: %s", err)
			return "", err
		}
		if set {
			s.RefreshSessionExpiry(sid)
			return s.ID(), nil
		} else {
			return "", nil
		}
	}

	if sessionNode == s.ID() {
		s.RefreshSessionExpiry(sid)
	}
	return sessionNode, err
}

func (s *redisSignalServer) RefreshSessionExpiry(sid string) {
	r := s.client
	r.Expire("node/"+s.ID(), SESSION_TIMEOUT)
	r.Expire("session/"+sid, SESSION_TIMEOUT)
}
