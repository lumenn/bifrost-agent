package services

import "encoding/json"

// InterfaceToJSON converts an interface{} to JSON bytes
func InterfaceToJSON(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

// JSONToStruct unmarshals JSON bytes into a struct
func JSONToStruct(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
