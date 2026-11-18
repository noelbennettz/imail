package signal

// InitChan signals the end of the initialization flow
var InitChan = make(chan bool)

// RestartChan triggers a program restart
var RestartChan = make(chan bool)

// StopChan triggers program shutdown
var StopChan = make(chan bool)
