package models

import "sync"

// Item struct used to store inventory items
type Item struct {
	Id              int     `json:"itemId"`
	ItemName        string  `json:"itemName"`
	ItemDescription string  `json:"itemDescription"`
	UnitPrice       float32 `json:"unitPrice"`
}

// InventoryView struct used to store Item
type Inventory struct {
	Items map[int]Item
	// A RWMutex used to access inventory global variable
	InventoryRWMutex sync.RWMutex
	NewId            int
}

// Struct of arguments of requests
type HandleClientRequestArguments struct {
	Method    string
	Variables []byte
}

// Struct of reply of requests
type HandleClientRequestReply struct {
	Result       bool
	ResponseData []byte
}
