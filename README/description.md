Marshal a struct-tagged Go value to JSON (e.g. [mapstructure](https://github.com/go-viper/mapstructure)-tagged) — reading the tags themselves, not a parallel set of `json` tags.

```go
type Settings struct {
	TabLen int  `mapstructure:"tab-len"`
	Fast   bool `mapstructure:"fast"`
}

func main() {
	m := tagjson.NewMarshaler("mapstructure")

	b, err := m.Marshal(Settings{TabLen: 4, Fast: false})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(b))
	// Output: {"tab-len":4}
}
```
