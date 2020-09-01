package main

type Device struct {
	Name     string
	Address  string
	PortSSH  uint16
	Username string
	Password string
	Key      string
	Timeout  int
	Every    int
}
