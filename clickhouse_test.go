package lucene

import (
	"strings"
	"testing"

	"github.com/AlxBystrov/go-lucene/pkg/driverclick"
)

func TestClickhouseSQLEndToEnd(t *testing.T) {
	type tc struct {
		input string
		want  string
		err   string
	}

	tcs := map[string]tc{
		// "single_literal": {
		// 	input: "a",
		// 	want:  `a`,
		// },
		"basic_equal": {
			input: "a:b",
			want:  `strings.value[indexOf(strings.name,'a')] = 'b'`,
		},
		"basic_equal_with_number": {
			input: "a:5",
			want:  `numbers.value[indexOf(numbers.name,'a')] = 5`,
		},
		"basic_greater_with_number": {
			input: "a:>22",
			want:  `numbers.value[indexOf(numbers.name,'a')] > 22`,
		},
		"basic_greater_eq_with_number": {
			input: "a:>=22",
			want:  `numbers.value[indexOf(numbers.name,'a')] >= 22`,
		},
		"basic_less_with_number": {
			input: "a:<22",
			want:  `numbers.value[indexOf(numbers.name,'a')] < 22`,
		},
		"basic_less_eq_with_number": {
			input: "a:<=22",
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 22`,
		},
		"basic_greater_less_with_number": {
			input: "a:<22 AND b:>33",
			want:  `(numbers.value[indexOf(numbers.name,'a')] < 22) AND (numbers.value[indexOf(numbers.name,'b')] > 33)`,
		},
		"basic_greater_less_eq_with_number": {
			input: "a:<=22 AND b:>=33",
			want:  `(numbers.value[indexOf(numbers.name,'a')] <= 22) AND (numbers.value[indexOf(numbers.name,'b')] >= 33)`,
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  `strings.value[indexOf(strings.name,'a')] like 'b%'`,
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  `strings.value[indexOf(strings.name,'a')] like 'b_z'`,
		},
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 5`,
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  `numbers.value[indexOf(numbers.name,'a')] < 5`,
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  `strings.value[indexOf(strings.name,'a')] BETWEEN 'foo' AND 'bar'`,
		},
		"basic_fuzzy": {
			input: "b AND a~",
			err:   "unable to render operator [FUZZY]",
		},
		"fuzzy_power": {
			input: "b AND a~10",
			err:   "unable to render operator [FUZZY]",
		},
		"basic_boost": {
			input: "b AND a^",
			err:   "unable to render operator [BOOST]",
		},
		"boost_power": {
			input: "b AND a^10",
			err:   "unable to render operator [BOOST]",
		},
		"regexp": {
			input: "a:/b [c]/",
			want:  `match(strings.value[indexOf(strings.name,'a')],'b [c]')`,
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want:  `match(strings.value[indexOf(strings.name,'a')],'b "[c]')`,
		},
		"regexp_with_escaped_chars": {
			input: `url:/example.com\/foo\/bar\/.*/`,
			want:  `match(strings.value[indexOf(strings.name,'url')],'example.com\/foo\/bar\/.*')`,
		},
		"basic_default_AND": {
			input: "a b",
			want:  `'a' AND 'b'`,
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			want:  `(strings.value[indexOf(strings.name,'a')] = 'b') AND (strings.value[indexOf(strings.name,'c')] = 'd')`,
		},
		"basic_and": {
			input: "a AND b",
			want:  `'a' AND 'b'`,
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			want:  `(strings.value[indexOf(strings.name,'a')] = 'foo') AND (strings.value[indexOf(strings.name,'b')] = 'bar')`,
		},
		"basic_or": {
			input: "a OR b",
			want:  `'a' OR 'b'`,
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			want:  `(strings.value[indexOf(strings.name,'a')] = 'foo') OR (strings.value[indexOf(strings.name,'b')] = 'bar')`,
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			want:  `numbers.value[indexOf(numbers.name,'a')] >= 1 AND numbers.value[indexOf(numbers.name,'a')] <= 5`,
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 200`,
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			want:  `strings.value[indexOf(strings.name,'a')] BETWEEN 'ab' AND 'az'`,
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			want:  `numbers.value[indexOf(numbers.name,'a')] > 2`,
		},
		"basic_not": {
			input: "NOT b",
			want:  `NOT('b')`,
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			want:  `(strings.value[indexOf(strings.name,'a')] = 'foo') OR (NOT(strings.value[indexOf(strings.name,'b')] = 'bar'))`,
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			want:  `((strings.value[indexOf(strings.name,'a')] = 'foo') OR (strings.value[indexOf(strings.name,'b')] = 'bar')) AND (strings.value[indexOf(strings.name,'c')] = 'baz')`,
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			want:  `strings.value[indexOf(strings.name,'a')] IN ('foo', 'baz', 'bar')`,
		},
		"basic_must": {
			input: "+a:b",
			want:  `strings.value[indexOf(strings.name,'a')] = 'b'`,
		},
		"basic_must_not": {
			input: "-a:b",
			want:  `NOT(strings.value[indexOf(strings.name,'a')] = 'b')`,
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			want:  `(strings.value[indexOf(strings.name,'d')] = 'e') AND ((NOT(strings.value[indexOf(strings.name,'a')] = 'b')) AND (strings.value[indexOf(strings.name,'f')] = 'e'))`,
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			want:  `strings.value[indexOf(strings.name,'a')] = '(1+1):2'`,
		},
		"escaped_column_name": {
			input: `foo\ bar:b`,
			want:  `strings.value[indexOf(strings.name,'foo bar')] = 'b'`,
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			err:   "unable to render operator [BOOST]",
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			want:  `(((strings.value[indexOf(strings.name,'title')] = 'foo') OR (strings.value[indexOf(strings.name,'title')] = 'bar')) AND ((strings.value[indexOf(strings.name,'body')] = 'foo') OR (strings.value[indexOf(strings.name,'body')] = 'bar'))) OR (strings.value[indexOf(strings.name,'k')] = 'v')`,
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			err:   "unable to render operator [FUZZY]",
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			want:  `(((strings.value[indexOf(strings.name,'a')] = 'b') AND (strings.value[indexOf(strings.name,'c')] = 'd')) OR (strings.value[indexOf(strings.name,'e')] = 'f')) OR ((strings.value[indexOf(strings.name,'h')] = 'i') AND (strings.value[indexOf(strings.name,'j')] = 'k'))`,
		},
		"test_precedence_weaving": {
			input: "a OR b AND c OR d",
			want:  `('a' OR ('b' AND 'c')) OR 'd'`,
		},
		"test_precedence_weaving_with_not": {
			input: "NOT a OR b AND NOT c OR d",
			want:  `((NOT('a')) OR ('b' AND (NOT('c')))) OR 'd'`,
		},
		"test_equals_in_precedence": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want:  `((strings.value[indexOf(strings.name,'a')] = 'az') OR ((strings.value[indexOf(strings.name,'b')] = 'bz') AND (NOT(strings.value[indexOf(strings.name,'c')] = 'z')))) OR 'd'`,
		},
		"test_parens_in_precedence": {
			input: "a AND (c OR d)",
			want:  `'a' AND ('c' OR 'd')`,
		},
		"test_range_precedence_simple": {
			input: "c:[* to -1] OR d",
			want:  `(numbers.value[indexOf(numbers.name,'c')] <= -1) OR 'd'`,
		},
		"test_range_precedence": {
			input: "a OR b AND c:[* to -1] OR d",
			want:  `('a' OR ('b' AND (numbers.value[indexOf(numbers.name,'c')] <= -1))) OR 'd'`,
		},
		"test_full_precedence": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want:  `('a' OR ('b' AND (numbers.value[indexOf(numbers.name,'c')] <= -1))) OR ('d' AND (NOT(strings.value[indexOf(strings.name,'e')] = 'f')))`,
		},
		"test_elastic_greater_than_precedence": {
			input: "a:>10 AND -b:<=-20",
			want:  `(numbers.value[indexOf(numbers.name,'a')] > 10) AND (NOT(numbers.value[indexOf(numbers.name,'b')] <= -20))`,
		},
		"escape_quotes": {
			input: "a:'b'",
			want:  `strings.value[indexOf(strings.name,'a')] = '''b'''`,
		},
		"name_starts_with_number": {
			input: "1a:b",
			want:  `strings.value[indexOf(strings.name,'1a')] = 'b'`,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			expr, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}

			got, err := driverclick.NewClickhouseDriver().Render(expr)
			if err != nil {
				// if we got an expect error then we are fine
				if tc.err != "" && strings.Contains(err.Error(), tc.err) {
					return
				}
				t.Fatalf("unexpected error rendering expression: %v", err)
			}

			if tc.err != "" {
				t.Fatalf("\nexpected error [%s]\ngot: %s", tc.err, got)
			}

			if got != tc.want {
				t.Fatalf("\nwant %s\ngot  %s\nparsed expression: %#v\n", tc.want, got, expr)
			}
		})
	}
}
