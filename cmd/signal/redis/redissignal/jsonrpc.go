package redissignal

import (
	"context"
	"encoding/json"
	"fmt"
	strings "strings"

	log "github.com/pion/ion-log"
	"github.com/pion/webrtc/v3"
	"github.com/sourcegraph/jsonrpc2"
)

type jsonRedisSignal struct {
	RedisSignal
}

func NewJSONRedisSignal(r RedisSignal) *jsonRedisSignal {
	return &jsonRedisSignal{r}
}

// Handle incoming RPC call events like join, answer, offer and trickle
func (s *jsonRedisSignal) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r := s.Redis()
	replyError := func(err error) {
		_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{
			Code:    500,
			Message: fmt.Sprintf("%s", err),
		})
	}

	switch req.Method {

	case "join":
		log.Infof("joining...")
		var join Join
		err := json.Unmarshal(*req.Params, &join)

		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}

		join.Pid = s.PeerID()

		msg, err := json.Marshal(join)

		go s.RPCPeerBus(ctx, conn, req)

		r.LPush("sfu/", msg)

	case "offer":
		var negotiation Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}
		// TODO: why is this wrapped in quotes?
		id := strings.Replace(req.ID.String(), "\"", "", -1)

		message, _ := json.Marshal(RPCCall{id, "offer", negotiation})
		r.LPush("peer-send/"+s.PeerID(), message)

	case "answer":
		var negotiation Negotiation
		err := json.Unmarshal(*req.Params, &negotiation)
		if err != nil {
			log.Errorf("connect: error parsing offer: %v", err)
			replyError(err)
			break
		}
		message, _ := json.Marshal(Notify{"answer", negotiation, "2.0"})
		r.LPush("peer-send/"+s.PeerID(), message)

	case "trickle":
		var trickle Trickle
		err := json.Unmarshal(*req.Params, &trickle)
		if err != nil {
			log.Errorf("connect: error parsing candidate: %v", err)
			replyError(err)
			break
		}
		if message, err := json.Marshal(Notify{"trickle", trickle, "2.0"}); err != nil {
			log.Errorf("error parsing message")
		} else {
			r.LPush("peer-send/"+s.PeerID(), message)
		}
	}

	//r.Expire("peer-send/"+s.PeerID(), 10*time.Second)
}

func (s *jsonRedisSignal) Close() {
	r := s.Redis()
	log.Infof("closing peer, sending kill message")
	r.LPush("peer-send/"+s.PeerID(), "kill")
	r.LPush("peer-recv/"+s.PeerID(), "kill")
}

func (s *jsonRedisSignal) RPCPeerBus(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r := s.Redis()
	topic := "peer-recv/" + s.PeerID()
	log.Infof("watch[%s] started", topic)

	for {

		message, err := r.BRPop(0, topic).Result()

		if err != nil {
			log.Errorf("unrecognized %s", message)
			continue
		}
		if message[1] == "kill" {
			s.Cleanup()
			return
		}

		var rpc ResultOrNotify

		if err := json.Unmarshal([]byte(message[1]), &rpc); err != nil {
			log.Errorf("failed to unmarshal rpc %s", message[1])
			continue
		}

		log.Infof("RPC: %s:%s/%s", rpc.ID, rpc.ResultType, rpc.Method)

		if rpc.ID != "" {
			conn.Reply(ctx, jsonrpc2.ID{Num: 0, Str: rpc.ID, IsString: true}, rpc.Result)
			continue
		}

		packed, err := json.Marshal(rpc.Params)
		if err != nil {
			log.Errorf("failed to marshal params %s", rpc.Params)
			continue
		}

		if rpc.ResultType == "answer" {
			var answer webrtc.SessionDescription
			if err := json.Unmarshal([]byte(message[1]), &answer); err != nil {
				log.Errorf("failed to unmarshal answer %s %s", err, message[1])
				continue
			}
			if err := conn.Reply(ctx, req.ID, answer); err != nil {
				log.Errorf("failed to send reply %s", err)
				continue
			}
		}

		if rpc.Method == "offer" {
			var offer webrtc.SessionDescription
			if err := json.Unmarshal(packed, &offer); err != nil {
				log.Errorf("failed to unmarshal answer %s %s", err, packed)
				continue
			}
			if err := conn.Notify(ctx, "offer", offer); err != nil {
				log.Errorf("error sending offer %s", err)
				continue
			}
		}

		if rpc.Method == "trickle" {
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(packed, &candidate); err != nil {
				log.Errorf("failed to unmarshal trickle %s %s", err, packed)
				continue
			}

			if err := conn.Notify(ctx, "trickle", candidate); err != nil {
				log.Errorf("error sending ice candidate %s", err)
				continue
			}
		}

	}
}
