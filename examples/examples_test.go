package examples

import (
	"fmt"

	"github.com/MarkRosemaker/tagjson"
)

func Example_mapstructure() {
	type Settings struct {
		TabLen int  `mapstructure:"tab-len"`
		Fast   bool `mapstructure:"fast"`
	}

	m := tagjson.NewMarshaler("mapstructure")

	b, err := m.Marshal(Settings{TabLen: 4, Fast: false})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(b))
	// Output: {"tab-len":4}
}

func Example_yaml() {
	type Settings struct {
		TabLen int  `yaml:"tab-len"`
		Fast   bool `yaml:"fast"`
	}

	m := tagjson.NewMarshaler("yaml")

	b, err := m.Marshal(Settings{TabLen: 4, Fast: false})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(b))
	// Output: {"tab-len":4}
}
