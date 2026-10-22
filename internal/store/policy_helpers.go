package store
import("encoding/json";"strings")
func decodeServices(raw string, out *[]string)error{return json.Unmarshal([]byte(raw),out)}
func splitPlatform(platform string)[]string{return strings.Split(platform,"/")}
