package firecracker

import(
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// BalloonBuilder is a builder that will create a balloon used to set up
// the firecracker microVM.
type BalloonBuilder struct {
	balloon models.Balloon
}

// NewBalloonBuilder will return a new BalloonBuilder with a given amountMB and DeflateOnOom.
func NewBalloonBuilder(AmountMb int64, DeflateOnOom bool) BalloonBuilder{
	return BalloonBuilder{}.createBalloon(AmountMb, DeflateOnOom)
}

// BalloonOpt represents an optional function used to allow for specific
// customization of the models.Balloon structure.
type BalloonOpt func(*models.Balloon)

// getBalloon return the balloon config
func (b BalloonBuilder) getBalloonConfig() models.Balloon {
	return b.balloon
}

// createBalloon will set the given balloon with new amountMB and DeflateOnOom. 
// The StatsPollingIntervals of host balloon will be set to 0 by default.
func (b BalloonBuilder) createBalloon(AmountMb int64, DeflateOnOom bool, opts ...BalloonOpt) BalloonBuilder {
	b.balloon = models.Balloon {
		AmountMb: &AmountMb,
		DeflateOnOom: &DeflateOnOom,
		StatsPollingIntervals: 0,
	}

	for _, opt := range opts {
		opt(&b.balloon)
	}

	return b
}

// updateBalloon will set the given balloon with new amountMB and DeflateOnOom. 
// The StatsPollingIntervals of host balloon will be set to 0 by default.
func (b BalloonBuilder) updateBalloon(AmountMb int64, opts ...BalloonOpt) BalloonBuilder {
	b.balloon = models.Balloon {
		AmountMb: &AmountMb,
	}

	for _, opt := range opts {
		opt(&b.balloon)
	}

	return b
}

// WithAmountMB sets the target size of the balloon
func WithAmountMB(amountMB int64) BalloonOpt {
	return func(d *models.Balloon) {
		d.AmountMb = &amountMB
	}
}

// WithDeflateOnOom sets the option for the balloon whether it should deflate when the guest has memory pressure.
func WithDeflateOnOom(deflateOnOom bool) BalloonOpt {
	return func(d *models.Balloon){
		d.DeflateOnOom = Bool(deflateOnOom)
	}
}

// WithStatsPollingIntervals sets the time in seconds between refreshing statistics. 
// A non-zero value will enable the statistics. Defaults to 0.
func WithStatsPollingIntervals(statsPollingIntervals int64) BalloonOpt {
	return func(d *models.Balloon){
		d.StatsPollingIntervals = statsPollingIntervals
	}
}

