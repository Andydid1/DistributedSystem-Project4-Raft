package models

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Handle inv request, return marshaled current inv
func (inv *Inventory) HandleInventory() []byte {
	inv.InventoryRWMutex.RLock()
	invByte, err := json.Marshal(inv)
	inv.InventoryRWMutex.RUnlock()
	// If error occurs, return empty []byte
	if err != nil {
		fmt.Println(err.Error())
		var emptyByte []byte
		return emptyByte
	}
	return invByte
}

// Handle add request
func (inv *Inventory) HandleAdd(requestDataByte []byte) {
	// Get new item
	var newItem Item
	unmarshalErr := json.Unmarshal(requestDataByte, &newItem)
	if unmarshalErr != nil {
		fmt.Println(unmarshalErr.Error())
		return
	}
	// Add new item and update global inv.NewId
	newItem.Id = inv.NewId
	inv.InventoryRWMutex.Lock()
	inv.Items[inv.NewId] = newItem
	inv.InventoryRWMutex.Unlock()
	inv.NewId += 1
}

// Handle update request
func (inv *Inventory) HandleUpdate(requestDataByte []byte) {
	// Get updated item
	var newItem Item
	unmarshalErr := json.Unmarshal(requestDataByte, &newItem)
	if unmarshalErr != nil {
		fmt.Println(unmarshalErr.Error())
		return
	}
	// If the id of the item to be update exists, update the item
	if _, ok := inv.Items[newItem.Id]; ok {
		inv.InventoryRWMutex.Lock()
		inv.Items[newItem.Id] = newItem
		inv.InventoryRWMutex.Unlock()
	}
}

// Handle delete request
func (inv *Inventory) HandleDelete(requestDataByte []byte) {
	// Get itemId
	var itemId int
	unmarshalErr := json.Unmarshal(requestDataByte, &itemId)
	if unmarshalErr != nil {
		fmt.Println(unmarshalErr.Error())
		return
	}
	// If the requested itemId exists, delete it
	if _, ok := inv.Items[itemId]; ok {
		inv.InventoryRWMutex.Lock()
		delete(inv.Items, itemId)
		inv.InventoryRWMutex.Unlock()
	}
}

// Handle non-read services
func (inv *Inventory) HandleNonRead(method string, requestDataByte []byte) error {
	// Forward the request to corresponding handler according to requested method
	switch method {
	case "add":
		inv.HandleAdd(requestDataByte)
	case "update":
		inv.HandleUpdate(requestDataByte)
	case "delete":
		inv.HandleDelete(requestDataByte)
	default:
		fmt.Printf("Requested method %s not recognized", method)
		return errors.New(fmt.Sprintf("Requested method %s not recognized", method))
	}
	return nil
}
