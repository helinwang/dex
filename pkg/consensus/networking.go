package consensus

// Network is the networking infrastructure.
type Networking struct {
}

func (n *Networking) Broadcast(data []byte) {
}

func (n *Networking) Send(addr Addr, data []byte) {
}

func (n *Networking) SendGroup(addrs []Addr) {
}
