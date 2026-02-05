//go:build llama

package service

// https://github.com/ggml-org/llama.cpp/blob/master/grammars/json.gbnf
const DefaultJSONObjectGrammar = `root ::= object
value ::= object | array | string | number | ("true" | "false" | "null") ws
object ::=
 "{" ws (
 string ":" ws value
 ("," ws string ":" ws value)*
 )? "}" ws
array ::=
 "[" ws (
 value
 ("," ws value)*
 )? "]" ws
string ::=
 "\"" (
 [^"\\\x7F\x00-\x1F] |
 "\\" (["\\bfnrt] | "u" [0-9a-fA-F]{4})
 )* "\"" ws
number ::= ("-"? ([0-9] | [1-9] [0-9]{0,15})) ("." [0-9]+)? ([eE] [-+]? [0-9] [1-9]{0,15})? ws
ws ::= | " " | "\n" [ \t]{0,20}
`
