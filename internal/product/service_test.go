package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTitle(main *testing.T) {
	main.Run("PrefersEnglish", func(t *testing.T) {
		s := NewService([]*Item{{ID: "widget", Title: map[string]string{"en": "Widget", "uk": "Віджет"}}})
		assert.Equal(t, "Widget", s.Title("widget"))
	})

	main.Run("FallsBackToAlphabeticallyFirstWhenEnglishMissing", func(t *testing.T) {
		s := NewService([]*Item{{ID: "widget", Title: map[string]string{"uk": "Віджет", "pl": "Pierdoła"}}})
		assert.Equal(t, "Pierdoła", s.Title("widget"))
	})

	main.Run("ReturnsEmptyWhenIDNotFound", func(t *testing.T) {
		s := NewService([]*Item{{ID: "widget", Title: map[string]string{"en": "Widget"}}})
		assert.Equal(t, "", s.Title("missing"))
	})

	main.Run("ReturnsEmptyWhenTitleMapEmpty", func(t *testing.T) {
		s := NewService([]*Item{{ID: "widget", Title: map[string]string{}}})
		assert.Equal(t, "", s.Title("widget"))
	})

	main.Run("SkipsEmptyEnglishValue", func(t *testing.T) {
		s := NewService([]*Item{{ID: "widget", Title: map[string]string{"en": "", "uk": "Віджет"}}})
		assert.Equal(t, "Віджет", s.Title("widget"))
	})
}
