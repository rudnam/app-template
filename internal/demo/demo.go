// Package demo fills a visitor's throwaway demo account with sample data,
// so they can see the app as it looks in use without making an account
// (DEMO, README.md). The account itself (no password, deleted after a day)
// is internal/account's; see its demo.go.
//
// It fills the example feature (notes). When you replace that, fill your
// own feature here instead, in both languages.
package demo

import (
	"context"
	"time"

	"github.com/francis/app-template/internal/notes"
)

// sample notes, oldest first, so the list shows the last at the top.
var sample = map[string][]string{
	"en": {
		"Welcome to the demo! Everything here is sample data, and it's deleted after a day.",
		"Groceries: eggs, rice, miso, spring onions",
		"Book the dentist for next month",
		"Idea: a weekly summary email",
	},
	"ja": {
		"デモへようこそ！ここにあるのはすべてサンプルで、1日後に削除されます。",
		"買い物: 卵、お米、味噌、ねぎ",
		"来月の歯医者を予約する",
		"アイデア: 週ごとのまとめメール",
	},
}

// Filler returns the function that fills a new demo account, in lang ("en"
// or "ja"). zone is the visitor's time zone, for anything dated.
func Filler(n *notes.Store) func(ctx context.Context, userID int64, lang string, zone *time.Location) error {
	return func(ctx context.Context, userID int64, lang string, zone *time.Location) error {
		bodies, ok := sample[lang]
		if !ok {
			bodies = sample["en"]
		}
		for _, body := range bodies {
			if _, err := n.Create(ctx, userID, body); err != nil {
				return err
			}
		}
		return nil
	}
}
