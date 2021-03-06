//  Copyright (c) 2013 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

// Plan is a description of a sequence of steps to produce a correct
// result for a query.

package simple

import (
	"fmt"

	"github.com/couchbaselabs/clog"
	"github.com/couchbaselabs/tuqtng/ast"
	"github.com/couchbaselabs/tuqtng/catalog"
	"github.com/couchbaselabs/tuqtng/plan"
	"github.com/couchbaselabs/tuqtng/planner"
)

func CanIUseThisIndexForThisProjectionNoWhereNoGroupClause(index catalog.RangeIndex, resultExprList ast.ResultExpressionList, bucket string) (bool, plan.ScanRanges, ast.Expression, error) {

	// convert the index key to formal notation
	indexKeyFormal, err := IndexKeyInFormalNotation(index.Key(), bucket)
	if err != nil {
		return false, nil, nil, err
	}

	// FIXME only looking at first element in key right now
	deps := ast.ExpressionList{indexKeyFormal[0]}
	clog.To(planner.CHANNEL, "index deps are: %v", deps)
	depChecker := ast.NewExpressionFunctionalDependencyCheckerFull(deps)

	// start looking at the projection
	allAggregateFunctionsMin := true
	for _, resultExpr := range resultExprList {

		// presence of * means we cannot use index on field, must see all (for this particular optimization)
		if resultExpr.Star {
			return false, nil, nil, nil
		}

		switch expr := resultExpr.Expr.(type) {
		case ast.AggregateFunctionCallExpression:
			_, isMin := expr.(*ast.FunctionCallMin)
			if !isMin {
				clog.To(planner.CHANNEL, "projection not MIN")
				allAggregateFunctionsMin = false
			}
			// aggregates all take 1 operand
			operands := expr.GetOperands()
			if len(operands) < 1 {
				return false, nil, nil, nil
			}
			aggOperand := operands[0]
			// preence of * means we cannot use this index, must see all (for this particular optimization)
			if aggOperand.Star {
				return false, nil, nil, nil
			}
			// look at dependencies inside this operand
			_, err := depChecker.Visit(aggOperand.Expr)
			if err != nil {
				return false, nil, nil, nil
			}
		default:
			// all expressions must be aggregates for this particular optimization
			return false, nil, nil, nil
		}
	}

	// if we made it this far, we can in fact use the index
	// doing a scan of all non-eliminatable items (non-NULL, non-MISSING)
	dummyOp := ast.NewIsNotNullOperator(indexKeyFormal[0])
	es := NewExpressionSargable(indexKeyFormal[0])
	dummyOp.Accept(es)
	if es.IsSargable() {
		ranges := es.ScanRanges()
		if allAggregateFunctionsMin {
			for _, r := range ranges {
				r.Limit = 1
			}
		}
		return true, ranges, nil, nil
	}
	clog.Error(fmt.Errorf("expected this to never happen"))

	// cannot use this index
	return false, nil, nil, nil
}

func CanIUseThisIndexForThisWhereClause(index catalog.RangeIndex, where ast.Expression, bucket string) (bool, plan.ScanRanges, ast.Expression, error) {

	// convert the index key to formal notation
	indexKeyFormal, err := IndexKeyInFormalNotation(index.Key(), bucket)
	if err != nil {
		return false, nil, nil, err
	}

	// put the where clause into conjunctive normal form
	ennf := ast.NewExpressionNNF()
	whereNNF, err := where.Accept(ennf)
	if err != nil {
		return false, nil, nil, err
	}
	ecnf := ast.NewExpressionCNF()
	whereCNF, err := whereNNF.Accept(ecnf)
	if err != nil {
		return false, nil, nil, err
	}

	switch whereCNF := whereCNF.(type) {
	case *ast.AndOperator:
		// this is an and, we can try to satisfy individual operands
		found := false
		rranges := plan.ScanRanges{}
		for _, oper := range whereCNF.Operands {
			// see if the where clause expression is sargable with respect to the index key
			es := NewExpressionSargable(indexKeyFormal[0])
			oper.Accept(es)
			if es.IsSargable() {
				found = true
				for _, ran := range es.ScanRanges() {
					rranges = MergeRanges(rranges, ran)
					clog.To(planner.CHANNEL, "now ranges are: %v", rranges)
				}
			}
		}
		if found {
			return true, rranges, nil, nil
		}
	default:
		// not an and, we must satisfy the whole expression
		// see if the where clause expression is sargable with respect to the index key
		es := NewExpressionSargable(indexKeyFormal[0])
		whereCNF.Accept(es)
		if es.IsSargable() {
			return true, es.ScanRanges(), nil, nil
		}
	}

	// cannot use this index
	return false, nil, nil, nil
}

func MergeRanges(origr plan.ScanRanges, newr *plan.ScanRange) plan.ScanRanges {
	rv := plan.ScanRanges{}

	merged := false
	for _, orig := range origr {

		if !merged {
			mergedRange := orig.Overlap(newr)
			if mergedRange != nil {
				merged = true
				rv = append(rv, mergedRange)

			} else {
				// not successful
				rv = append(rv, orig)
			}
		} else {
			// already merged, just append the orig
			rv = append(rv, orig)
		}
	}

	// never merged it
	if !merged {
		rv = append(rv, newr)
	}

	return rv
}

func IndexKeyInFormalNotation(key catalog.IndexKey, bucket string) (catalog.IndexKey, error) {
	fkey := make(catalog.IndexKey, len(key))
	fnot := ast.NewExpressionFormalNotationConverter([]string{}, []string{bucket}, bucket)
	for i, kp := range key {
		kpc := kp.Copy()
		nkey, err := kpc.Accept(fnot)
		if err != nil {
			return nil, err
		}
		fkey[i] = nkey
	}
	return fkey, nil
}

func DoesIndexCoverStatement(index catalog.RangeIndex, stmt *ast.SelectStatement) bool {

	if stmt.From.Over != nil {
		// index cannot cover queries containing OVER right now
		return false
	}

	// convert the index key to formal notation
	indexKeyFormal, err := IndexKeyInFormalNotation(index.Key(), stmt.From.As)
	if err != nil {
		return false
	}

	deps := ast.ExpressionList{}
	for _, indexKey := range indexKeyFormal {
		deps = append(deps, indexKey)
	}
	clog.To(planner.CHANNEL, "index deps are: %v", deps)
	depChecker := ast.NewExpressionFunctionalDependencyCheckerFull(deps)

	// first check the projection
	for _, resultExpr := range stmt.Select {
		if resultExpr.Star == true || resultExpr.Expr == nil {
			// currently cannot cover *
			return false
		}

		_, err := depChecker.Visit(resultExpr.Expr)
		if err != nil {
			return false
		}
	}

	if stmt.Where != nil {
		_, err = depChecker.Visit(stmt.Where)
		if err != nil {
			return false
		}
	}

	if stmt.GroupBy != nil {
		for _, groupExpr := range stmt.GroupBy {
			_, err = depChecker.Visit(groupExpr)
			if err != nil {
				return false
			}
		}

		if stmt.Having != nil {
			_, err = depChecker.Visit(stmt.Having)
			if err != nil {
				return false
			}
		}
	}

	if stmt.OrderBy != nil {
		for _, orderExpr := range stmt.OrderBy {
			_, err = depChecker.Visit(orderExpr.Expr)
			if err != nil {
				return false
			}
		}
	}

	// if we go this far it is covered
	return true
}
