// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"fmt"
	"go/types"

	"github.com/s48/transform/cps"
)

func convertTypes(call *cps.CallNodeT) {
	for _, vart := range call.Outputs {
		if vart != nil && vart.Type != nil {
			vart.Type = convertType(vart.Type)
		}
	}
	for _, input := range call.Inputs {
		if cps.IsLiteralNode(input) {
			node := input.(*cps.LiteralNodeT)
			if node.Type == nil {
				panic("literal has nil type")
			}
			_, converted := node.Type.(nativeTypeT)
			if !converted {
				node.Type = convertType(node.Type)
			}
		}
	}
	for _, next := range call.Next {
		convertTypes(next)
	}
}

var basicTypes = map[types.BasicKind]nativeTypeT{
	types.Bool:        theBoolType,
	types.UntypedBool: theBoolType,
	types.UntypedInt:  theInt64Type,
	types.Int8:        theInt8Type,
	types.Uint8:       theUint8Type,
	types.Int16:       theInt16Type,
	types.Uint16:      theUint16Type,
	types.Int32:       theInt32Type,
	types.Uint32:      theUint32Type,
	types.Int:         theInt64Type,
	types.Int64:       theInt64Type,
	types.Uint:        theUint64Type,
	types.Uint64:      theUint64Type,
}

func convertType(rawGoType types.Type) nativeTypeT {
	switch goType := rawGoType.(type) {
	case *types.Basic:
		nativeType, found := basicTypes[goType.Kind()]
		if found {
			return nativeType
		}
		panicf("can't convert Go type '%s'", goType)
	case *types.Tuple:
		native := &nativeTupleT{make([]nativeTypeT, goType.Len())}
		for i := range goType.Len() {
			native.contents[i] = convertType(goType.At(i).Type())
		}
		return native
	case *types.Pointer:
		return &nativePointerT{convertType(goType.Elem())}
	default:
		panicf("Can't convert Go type: '%s' with type %T", goType, goType)
	}
	return nil
}

//----------------------------------------------------------------
// Native types

type nativeTypeKindT int

const (
	boolType = iota
	int8Type
	uint8Type
	int16Type
	uint16Type
	int32Type
	uint32Type
	int64Type
	uint64Type
	pointerType
	tupleType // only used for return values
)

type nativeTypeT interface {
	kind() nativeTypeKindT
	sizeBytes() int

	// Implement the types.Type interface.
	Underlying() types.Type
	String() string
}

type atomicTypeT struct {
	nativeKind nativeTypeKindT
	name       string
	size       int
}

func (atomic *atomicTypeT) kind() nativeTypeKindT  { return atomic.nativeKind }
func (atomic *atomicTypeT) sizeBytes() int         { return atomic.size }
func (atomic *atomicTypeT) Underlying() types.Type { return atomic }
func (atomic *atomicTypeT) String() string         { return atomic.name }

var theBoolType = &atomicTypeT{boolType, "bool", 1}
var theInt8Type = &atomicTypeT{int8Type, "int8", 1}
var theUint8Type = &atomicTypeT{uint8Type, "uint8", 1}
var theInt16Type = &atomicTypeT{int16Type, "int16", 2}
var theUint16Type = &atomicTypeT{uint16Type, "uint16", 2}
var theInt32Type = &atomicTypeT{int32Type, "int32", 4}
var theUint32Type = &atomicTypeT{uint32Type, "uint32", 4}
var theInt64Type = &atomicTypeT{int64Type, "int64", 8}
var theUint64Type = &atomicTypeT{uint64Type, "uint64", 8}

type nativePointerT struct {
	contents nativeTypeT
}

func (pointer *nativePointerT) kind() nativeTypeKindT  { return pointerType }
func (pointer *nativePointerT) sizeBytes() int         { return 8 }
func (pointer *nativePointerT) Underlying() types.Type { return pointer }
func (pointer *nativePointerT) String() string         {
	return fmt.Sprintf("pointer(%s)", pointer.contents)
}

type nativeTupleT struct {
	contents []nativeTypeT
}

func (tuple *nativeTupleT) kind() nativeTypeKindT  { return tupleType }
func (tuple *nativeTupleT) sizeBytes() int         { return 0 }
func (tuple *nativeTupleT) Underlying() types.Type { return tuple }
func (tuple *nativeTupleT) String() string         { return "tuple" }
