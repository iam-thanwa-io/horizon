package horizon

import (
	"errors"

	"fmt"

	"github.com/stellar/go/xdr"
	"github.com/stellar/horizon/db2"
	"github.com/stellar/horizon/db2/history"
	"github.com/stellar/horizon/render/hal"
	"github.com/stellar/horizon/resource"
)

// TradeIndexAction renders a page of effect resources, filtered to include
// only trades, identified by a normal page query and optionally filtered by an account
// or order book
type TradeIndexAction struct {
	Action
	AccountFilter string
	Selling       xdr.Asset
	Buying        xdr.Asset
	PagingParams  db2.PageQuery
	Records       []history.Effect
	// LedgerRecords is a cache of loaded ledger data used to populate the time
	// when a trade occurred.
	LedgerRecords history.LedgerMap
	Page          hal.Page
}

// JSON is a method for actions.JSON
func (action *TradeIndexAction) JSON() {
	action.Do(
		action.EnsureHistoryFreshness,
		action.loadParams,
		action.loadRecords,
		action.loadLedgers,
		action.loadPage,
		func() {
			hal.Render(action.W, action.Page)
		},
	)
}

// LoadQuery sets action.Query from the request params
func (action *TradeIndexAction) loadParams() {
	action.AccountFilter = action.GetString("account_id")
	action.PagingParams = action.GetPageQuery()

	// scott: It is unfortunate that we have this string guard below.  Instead, we
	// should probably add an alternative to `GetAsset` that returns a zero-value
	// xdr.Asset when not provided by the request.  Perhaps `MaybeGetAsset`?.
	if action.GetString("selling_asset_type") != "" {
		action.Selling = action.GetAsset("selling_")
		action.Buying = action.GetAsset("buying_")
	}
}

// loadRecords populates action.Records
func (action *TradeIndexAction) loadRecords() {
	trades := action.HistoryQ().Effects().OfType(history.EffectTrade)

	if action.AccountFilter != "" {
		trades = trades.ForAccount(action.AccountFilter)
	}

	if (action.Selling != xdr.Asset{} || action.Buying != xdr.Asset{}) {
		trades = trades.ForOrderBook(action.Selling, action.Buying)
	}

	action.Err = trades.Page(action.PagingParams).Select(&action.Records)
}

// loadLedgers collects the unique ledgers referenced in the loaded trades and loads the details for each.
func (action *TradeIndexAction) loadLedgers() {
	loader := history.LedgerCache{
		DB: action.HistoryQ(),
	}

	sequences := make([]history.LedgerSequencer, len(action.Records))
	for i, r := range action.Records {
		sequences[i] = &action.Records[i]
	}

	action.LedgerRecords, action.Err = loader.LoadLedgers(sequences)
}

// loadPage populates action.Page
func (action *TradeIndexAction) loadPage() {
	for _, record := range action.Records {
		var res resource.Trade

		ledger, found := action.LedgerRecords[record.GetLedgerSequence()]
		if !found {
			msg := fmt.Sprintf("could not find ledger data for sequence %d", record.GetLedgerSequence())
			action.Err = errors.New(msg)
			return
		}

		action.Err = res.Populate(action.Ctx, record, ledger)
		if action.Err != nil {
			return
		}

		action.Page.Add(res)
	}

	action.Page.BaseURL = action.BaseURL()
	action.Page.BasePath = action.Path()
	action.Page.Limit = action.PagingParams.Limit
	action.Page.Cursor = action.PagingParams.Cursor
	action.Page.Order = action.PagingParams.Order
	action.Page.PopulateLinks()
}
