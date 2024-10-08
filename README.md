# basic-raft

Implementation of Raft algorithm made with Go for fun and learning

### Manual testing (Acceptance test)

1. Start 2 nodes => the Leader should be elected

```bash
> docker-compose up node1 node2
...
> curl http://localhost:80/api/v1/state
{"term":1,"state":"FOLLOWER","log":[],"uncommitted":[]}
> curl http://localhost:81/api/v1/state 
{"term":1,"state":"LEADER","log":[],"uncommitted":[]}
```

2. Post `msg1`, `msg2` => messages should be replicated and committed

```bash
> curl -X POST http://localhost:81/api/v1/append -H "Content-Type: application/json" -d '{"command":"msg1"}'
> curl -X POST http://localhost:81/api/v1/append -H "Content-Type: application/json" -d '{"command":"msg2"}'
> curl http://localhost:80/api/v1/state
{"term":1,"state":"FOLLOWER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[]}
> curl http://localhost:81/api/v1/state 
{"term":1,"state":"LEADER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[]}
```

3. Start 3-rd node => messages should be replicated on the 3-rd node

```bash
> docker-compose up node3
...
> curl http://localhost:82/api/v1/state
{"term":1,"state":"FOLLOWER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[]}
```

4. Partition a Leader - (`OldLeader`) => a `NewLeader` should be elected

```bash
> docker network disconnect deployment_isolated_network deployment_node2_1
> curl http://localhost:80/api/v1/state
{"term":2,"state":"LEADER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[]}
> curl http://localhost:82/api/v1/state
{"term":2,"state":"FOLLOWER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[]}
```

5. Post `msg3`, `msg4` via `NewLeader` => messages should be replicated and committed

```bash
> curl -X POST http://localhost:80/api/v1/append -H "Content-Type: application/json" -d '{"command":"msg3"}'
> curl -X POST http://localhost:80/api/v1/append -H "Content-Type: application/json" -d '{"command":"msg4"}'
> curl http://localhost:80/api/v1/state
{"term":2,"state":"LEADER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"},{"term":2,"command":"msg3"},{"term":2,"command":"msg4"}],"uncommitted":[]}
> curl http://localhost:82/api/v1/state
{"term":2,"state":"FOLLOWER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"},{"term":2,"command":"msg3"},{"term":2,"command":"msg4"}],"uncommitted":[]}
```

6. Post `msg5` via `OldLeader` => message should not be committed

```bash
> docker exec -it deployment_node2_1 curl -X POST http://localhost:8000/api/v1/append -H "Content-Type: application/json"  -d '{"command":"msg5"}' --max-time 2
curl: (28) Operation timed out after 2001 milliseconds with 0 bytes received 
^^^ this means that message is not committed
> docker exec -it deployment_node2_1 curl http://localhost:8000/api/v1/state
{"term":1,"state":"LEADER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"}],"uncommitted":[{"term":1,"command":"msg5"}]}
```

7. Join cluster => `msg5` on the `OldLeader` should be replaced by messages from the `NewLeader`

```bash
> docker network connect deployment_isolated_network --alias node2 deployment_node2_1
> curl http://localhost:81/api/v1/state 
{"term":2,"state":"FOLLOWER","log":[{"term":1,"command":"msg1"},{"term":1,"command":"msg2"},{"term":2,"command":"msg3"},{"term":2,"command":"msg4"}],"uncommitted":[]}
```
