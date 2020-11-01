package redissignal

import (
	"log"
	"os"
	"testing"

	"github.com/alicebob/miniredis"
	"github.com/elliotchance/redismock"
	"github.com/go-redis/redis"
	sfu "github.com/pion/ion-sfu/pkg"
	"github.com/stretchr/testify/assert"
)

var (
	client *redis.Client
	ion    sfu.SFU
)

var (
	nid   = "node123"
	sid   = "test-session"
	pid   = "99-88-77"
	offer = "NEED REAL OFFER HERE"
)

func NewTestServer(client redis.Cmdable) RedisSignalServer {
	return NewRedisSignalServer(ion, client, nid)
}

func NewTestSignal(client redis.Cmdable) RedisSignal {
	s := NewTestServer(client)
	p := sfu.NewPeer(&ion)
	return NewRedisSignal(p, s, pid)
}

func TestMain(m *testing.M) {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	client = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ion = *sfu.NewSFU(sfu.Config{})

	code := m.Run()
	os.Exit(code)
}

func TestSessionDoesntExist(t *testing.T) {
	mock := redismock.NewNiceMock(client)
	mock.On("Get", SessionKey(sid)).Return(redis.NewStringResult("", nil))

	s := NewTestSignal(mock)
	exists_false, err := s.SessionExists(sid)

	assert.NoError(t, err)
	assert.False(t, exists_false)
}

func TestSessionExists(t *testing.T) {
	mock := redismock.NewNiceMock(client)
	mock.On("Get", SessionKey(sid)).Return(redis.NewStringResult("1", nil))

	s := NewTestSignal(mock)
	exists_true, err := s.SessionExists(sid)

	assert.NoError(t, err)
	assert.True(t, exists_true)
}

func TestAquireSessionLock(t *testing.T) {
	mock := redismock.NewNiceMock(client)
	mock.On("Incr", SessionKey(sid)).Return(redis.NewIntResult(1, nil))

	s := NewTestSignal(mock)

	aquire_lock, err := s.AttemptSessionLock(sid)

	assert.NoError(t, err)
	assert.True(t, aquire_lock)
}

func TestCantAquireSessionLock(t *testing.T) {
	mock := redismock.NewNiceMock(client)
	mock.On("Incr", SessionKey(sid)).Return(redis.NewIntResult(2, nil))

	s := NewTestSignal(mock)

	aquire_lock, err := s.AttemptSessionLock(sid)

	assert.NoError(t, err)
	assert.False(t, aquire_lock)
}
