//  Copyright (c) 2013 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
//  except in compliance with the License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing, software distributed under the
//  License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
//  either express or implied. See the License for the specific language governing permissions
//  and limitations under the License.

package goyacc

import (
	"encoding/json"
	"log"
	"reflect"
	"testing"

	"github.com/couchbaselabs/tuqtng/ast"
)

var validQueries = []string{
	// this section is all basic expression testing
	`SELECT null`,
	`SELECT NULL`,
	`SELECT NuLl`,
	`SELECT true`,
	`SELECT TRUE`,
	`SELECT tRuE`,
	`SELECT false`,
	`SELECT FALSE`,
	`SELECT fAlSe`,
	`SELECT 1`,
	`SELECT -3`,
	`SELECT 1.5`,
	`SELECT -3.14`,
	`SELECT 1e6`,
	`SELECT 1.3e23`,
	`SELECT -4e-4`,
	`SELECT []`,
	`SELECT [null, false, true, 7, 3.14, "bob"]`,
	`SELECT {}`,
	`SELECT {"bob": "wood"}`,
	`SELECT {"bob": 1}`,
	`SELECT {"null": null, "bool": true, "number": 7, "array": [2, 3, "cat"], "object": {"nested": 99}}`,
	"SELECT `bob wood`",
	`SELECT 3 > 7`,
	`SELECT 1 < 2`,
	`SELECT 1 == 3`,
	`SELECT 2 = 4`,
	`SELECT 3 <> cat`,
	`SELECT dog != cat`,
	`SELECT wow >= what`,
	`SELECT [] <= 7`,
	`SELECT a + b`,
	`SELECT d - c`,
	`SELECT x * y`,
	`SELECT z / 2`,
	`SELECT minutes % 60`,
	`SELECT str1 || str2`,
	`SELECT (3 + c) * 7`,
	`SELECT cat IS NULL`,
	`SELECT dog IS NOT NULL`,
	`SELECT steve IS MISSING`,
	`SELECT gerald IS NOT MISSING`,
	`SELECT siri IS VALUED`,
	`SELECT marty IS NOT VALUED`,
	`SELECT noone LIKE them`,
	`SELECT someone NOT LIKE me`,
	`SELECT -abv`,
	`SELECT contact.name`,
	`SELECT contact.name.first`,
	`SELECT {"bob": "wood"}.wood`,
	`SELECT family["father"]`,
	`SELECT [a,b,c][0]`,
	`SELECT [0,[1,2,[3,4,5]]][1][2][1]`,
	`SELECT cat.a+b`,  // DOT has higher precedance than PLUS so this is valid
	`SELECT cat[a+b]`, // if you wanted to evaluate a+b and use that as property name
	// more complicated query testing
	`SELECT bob FROM cat WHERE foo = bar ORDER BY this LIMIT 10 OFFSET 20`,
	`SELECT bob FROM cat WHERE foo = bar LIMIT 10 OFFSET 20`,
	`SELECT bob FROM cat WHERE foo = bar LIMIT 10`,
	`SELECT bob FROM cat WHERE foo = bar`,
	`SELECT bob FROM cat`,
	`SELECT bob FROM cat WHERE foo = bar and 3 > 4`,
	`SELECT bob, bill FROM cat WHERE foo = bar and 3 > 4`,
	`SELECT bob AS bill, bill AS bob FROM cat WHERE foo = bar and 3 > 4`,
	`SELECT *, bob AS bill, bill AS bob FROM cat WHERE foo = bar and 3 > 4`,
	`SELECT *, names.*, bob AS bill, bill AS bob FROM cat WHERE foo = bar and 3 > 4`,
}

var invalidQueries = []string{
	`bob`,         // must have select
	`SELECT 01`,   // numbers cannot start with leading zeros
	`SELECT 3dog`, // unescaped identifiers cannot start with number
	`SELECT bob FROM cat WHERE foo = bar ORDER BY this OFFSET 20`,                  // offset requires limit
	`SELECT * AS all, bob AS bill, bill AS bob FROM cat WHERE foo = bar and 3 > 4`, // cannot alias *
}

func TestParser(t *testing.T) {
	DebugTokens = false
	DebugGrammar = false
	unqlParser := NewUnqlParser()

	for _, v := range validQueries {
		_, err := unqlParser.Parse(v)
		if err != nil {
			t.Errorf("Valid Query Parse Failed: %v - %v", v, err)
			//enable debugging and run it again
			log.Printf("Debug for input: %v", v)
			DebugTokens = true
			DebugGrammar = true
			unqlParser.Parse(v)
			DebugTokens = false
			DebugGrammar = false
		}
	}

	for _, v := range invalidQueries {
		_, err := unqlParser.Parse(v)
		if err == nil {
			t.Errorf("Invalid Query Parsed Successfully: %v - %v", v, err)
		}
	}

}

func TestParserASTOutput(t *testing.T) {

	tests := []struct {
		input  string
		output *ast.SelectStatement
	}{
		{"SELECT * FROM test WHERE true",
			&ast.SelectStatement{
				Select: ast.ResultExpressionList{
					ast.NewStarResultExpression(),
				},
				From:  nil,
				Where: ast.NewLiteralBool(true),
			},
		},
		{"SELECT * FROM test ORDER BY foo",
			&ast.SelectStatement{
				Select: ast.ResultExpressionList{
					ast.NewStarResultExpression(),
				},
				From:  nil,
				Where: nil,
				OrderBy: []*ast.SortExpression{
					ast.NewSortExpression(ast.NewProperty("foo"), true),
				},
			},
		},
		{"SELECT * FROM test LIMIT 10 OFFSET 3",
			&ast.SelectStatement{
				Select: ast.ResultExpressionList{
					ast.NewStarResultExpression(),
				},
				From:   nil,
				Where:  nil,
				Limit:  10,
				Offset: 3,
			},
		},
		{"SELECT a FROM test",
			&ast.SelectStatement{
				Select: ast.ResultExpressionList{
					ast.NewResultExpression(ast.NewProperty("a")),
				},
				From:  nil,
				Where: nil,
			},
		},
	}

	DebugTokens = false
	DebugGrammar = false
	unqlParser := NewUnqlParser()

	for _, v := range tests {
		query, err := unqlParser.Parse(v.input)
		if err != nil {
			t.Errorf("Valid Query Parse Failed: %v - %v", v, err)
		}

		if !reflect.DeepEqual(query, v.output) {
			t.Errorf("Expected %v, got %v", v.output, query)

			json, err := json.Marshal(query)
			if err == nil {
				log.Printf("%v", string(json))
			}
		}
	}

}