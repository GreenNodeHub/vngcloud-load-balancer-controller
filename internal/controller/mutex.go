package controller

import "sync"

// Global mutex to control security group updates, avoid update conflicts
var secGroupMutex sync.Mutex

// Global mutex to control update tags, avoid update conflicts
var tagMutex sync.Mutex
