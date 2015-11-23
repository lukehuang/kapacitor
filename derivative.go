package kapacitor

import (
	"time"

	"github.com/influxdb/kapacitor/models"
	"github.com/influxdb/kapacitor/pipeline"
)

type DerivativeNode struct {
	node
	d *pipeline.DerivativeNode
}

// Create a new derivative node.
func newDerivativeNode(et *ExecutingTask, n *pipeline.DerivativeNode) (*DerivativeNode, error) {
	dn := &DerivativeNode{
		node: node{Node: n, et: et},
		d:    n,
	}
	// Create stateful expressions
	dn.node.runF = dn.runDerivative
	return dn, nil
}

func (d *DerivativeNode) runDerivative() error {
	switch d.Provides() {
	case pipeline.StreamEdge:
		previous := make(map[models.GroupID]models.Point)
		for p, ok := d.ins[0].NextPoint(); ok; p, ok = d.ins[0].NextPoint() {
			pr, ok := previous[p.Group]
			if !ok {
				previous[p.Group] = p
				continue
			}

			value, ok := d.derivative(pr.Fields, p.Fields, pr.Time, p.Time)
			if ok {
				fields := pr.Fields.Copy()
				fields[d.d.As] = value
				pr.Fields = fields
				for _, child := range d.outs {
					err := child.CollectPoint(pr)
					if err != nil {
						return err
					}
				}
			}
			previous[p.Group] = p
		}
	case pipeline.BatchEdge:
		for b, ok := d.ins[0].NextBatch(); ok; b, ok = d.ins[0].NextBatch() {
			if len(b.Points) > 0 {
				pr := b.Points[0]
				var p models.BatchPoint
				for i := 1; i < len(b.Points); i++ {
					p = b.Points[i]
					value, ok := d.derivative(pr.Fields, p.Fields, pr.Time, p.Time)
					if ok {
						fields := pr.Fields.Copy()
						fields[d.d.As] = value
						b.Points[i-1].Fields = fields
					} else {
						b.Points = append(b.Points[:i-1], b.Points[i:]...)
						i--
					}
					pr = p
				}
				b.Points = b.Points[:len(b.Points)-1]
			}
			for _, child := range d.outs {
				err := child.CollectBatch(b)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *DerivativeNode) derivative(prev, curr models.Fields, prevTime, currTime time.Time) (float64, bool) {
	f0, ok := prev[d.d.Field].(float64)
	if !ok {
		return 0, false
	}

	f1, ok := curr[d.d.Field].(float64)
	if !ok {
		return 0, false
	}

	elapsed := currTime.Sub(prevTime)
	diff := f1 - f0
	// Drop negative values for non-negative derivatives
	if d.d.NonNegativeFlag && diff < 0 {
		return 0, false
	}

	value := float64(diff) / (float64(elapsed) / float64(d.d.Unit))
	return value, true
}