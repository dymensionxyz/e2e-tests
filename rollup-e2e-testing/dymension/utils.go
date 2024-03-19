package dymension

import (
	"strconv"

	"github.com/decentrio/rollup-e2e-testing/blockdb"
)

func MapToEibcEvent(event blockdb.Event) (EibcEvent, error) {
	var eibcEvent EibcEvent

	for _, attr := range event.Attributes {
		switch attr.Key {
		case "id":
			eibcEvent.ID = attr.Value
		case "price":
			eibcEvent.Price = attr.Value
		case "fee":
			eibcEvent.Fee = attr.Value
		case "is_fulfilled":
			isFulfilled, err := strconv.ParseBool(attr.Value)
			if err != nil {
				return EibcEvent{}, err
			}
			eibcEvent.IsFulfilled = isFulfilled
		case "packet_status":
			eibcEvent.PacketStatus = attr.Value
		}
	}

	return eibcEvent, nil
}
