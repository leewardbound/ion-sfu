# redis signaling

*** NOT FUNCTIONAL YET *** 

* a Redis signaling interface for ion-sfu, send and recieve signaling messages with redis message queues

* send join messages on topic `sfu/`, many SFU nodes will bid to lock the session; you MUST add a `pid` to your join params

* send all following messages on `peer-send/{pid}` and listen on `peer-recv/{pid}` for replies

* follows the `jsonrpc` api, uses  `jsonrpc`-compatible messages like `{"method": "join", "params"...}`


### JSONRPC-redis bridge

*** ONLY USE FOR TESTING ***

* passing `-j :7000` as a CLI argument will start a jsonrpc server on `:7000`, sending all messages to redis

* compatible with echotest and pubsubtest

* if a client connects to sfu `1.1.1.1:7000`, but the session is on SFU `5.5.5.5`, the messages should pass
    transparently through redis
