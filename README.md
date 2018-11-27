# doogle 

_Web search of the people, by the people, for the people with Go._

[![CircleCI](https://circleci.com/gh/mathetake/doogle.svg?style=shield)](https://circleci.com/gh/mathetake/doogle)
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)


doogle is a PoC implementation of __decentralized search engine__ based on gRPC written in Go.

For PoC purposes only. __NOT__ to use in production environment.

## algorithms behind doogle

### Distributed Hash Table based on S/Kademlia
Baumgart, Ingmar, and Sebastian Mies. "S/kademlia: A practicable approach towards secure key-based routing." Parallel and Distributed Systems, 2007 International Conference on. IEEE, 2007.

### local estimation of PageRank with `WorldNode`
Parreira, Josiane Xavier, et al. "Efficient and decentralized pagerank approximation in a peer-to-peer web search network." Proceedings of the 32nd international conference on Very large data bases. VLDB Endowment, 2006.


## development

- install go, grpc, protc, go-grpc, etc.

- if you modify doogle.proto, then run:

```bash
protoc -I grpc/ grpc/doogle.proto --go_out=plugins=grpc:grpc
```

## References

see my survey: [Towards decentralized information retrieval: research papers](https://github.com/mathetake/notes/issues/1)


Also there are two articles in __Japanese__:
- [Distributed Hash Table and p2p Search Engine](https://scrapbox.io/layerx/Distributed_Hash_Table_and_p2p_Search_Engine)
- [Peer-to-Peer Information Retrieval: An Overview](https://scrapbox.io/layerx/%5BWIP%5DPeer-to-Peer_Information_Retrieval:_An_Overview)

## LICENSE

MIT
