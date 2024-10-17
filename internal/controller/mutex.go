package controller

import "sync"

// Global mutex to control security group updates, avoid update conflicts
var secGroupMutex sync.Mutex
