/*
 * Cadence - The resource-oriented smart contract programming language
 *
 * Copyright Flow Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package interpreter

import (
	"fmt"

	"github.com/onflow/atree"

	"github.com/onflow/cadence/errors"
	"github.com/onflow/cadence/values"
)

// TODO: remove once migrated

// Deprecated: LinkValue
type LinkValue interface {
	Value
	isLinkValue()
}

// Deprecated: PathLinkValue
type PathLinkValue struct {
	Type       StaticType
	TargetPath PathValue
}

var EmptyPathLinkValue = PathLinkValue{}

var _ Value = PathLinkValue{}
var _ atree.Value = PathLinkValue{}
var _ EquatableValue = PathLinkValue{}
var _ LinkValue = PathLinkValue{}

func (PathLinkValue) IsValue() {}

func (PathLinkValue) isLinkValue() {}

func (v PathLinkValue) Accept(context ValueVisitContext, visitor Visitor, locationRange LocationRange) {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) Walk(_ ValueWalkContext, _ func(Value), _ LocationRange) {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) StaticType(context ValueStaticTypeContext) StaticType {
	// When iterating over public/private paths,
	// the values at these paths are PathLinkValues,
	// placed there by the `link` function.
	//
	// These are loaded as links, however,
	// for the purposes of checking their type,
	// we treat them as capabilities
	return NewCapabilityStaticType(context, v.Type)
}

func (PathLinkValue) IsImportable(_ ValueImportableContext, _ LocationRange) bool {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) String() string {
	return v.RecursiveString(SeenReferences{})
}

func (v PathLinkValue) RecursiveString(seenReferences SeenReferences) string {
	return fmt.Sprintf(
		"PathLink<%s>(%s)",
		v.Type.String(),
		v.TargetPath.RecursiveString(seenReferences),
	)
}

func (v PathLinkValue) MeteredString(_ ValueStringContext, _ SeenReferences, _ LocationRange) string {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) ConformsToStaticType(
	_ ValueStaticTypeConformanceContext,
	_ LocationRange,
	_ TypeConformanceResults,
) bool {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) Equal(context ValueComparisonContext, locationRange LocationRange, other Value) bool {
	otherLink, ok := other.(PathLinkValue)
	if !ok {
		return false
	}

	return otherLink.TargetPath.Equal(context, locationRange, v.TargetPath) &&
		otherLink.Type.Equal(v.Type)
}

func (PathLinkValue) IsStorable() bool {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) Storable(storage atree.SlabStorage, address atree.Address, maxInlineSize uint64) (atree.Storable, error) {
	return values.MaybeLargeImmutableStorable(v, storage, address, maxInlineSize)
}

func (PathLinkValue) NeedsStoreTo(_ atree.Address) bool {
	panic(errors.NewUnreachableError())
}

func (PathLinkValue) IsResourceKinded(context ValueStaticTypeContext) bool {
	panic(errors.NewUnreachableError())
}

func (v PathLinkValue) Transfer(
	context ValueTransferContext,
	_ LocationRange,
	_ atree.Address,
	remove bool,
	storable atree.Storable,
	_ map[atree.ValueID]struct{},
	_ bool,
) Value {
	if remove {
		RemoveReferencedSlab(context, storable)
	}
	return v
}

func (v PathLinkValue) Clone(context ValueCloneContext) Value {
	return PathLinkValue{
		Type:       v.Type,
		TargetPath: v.TargetPath.Clone(context).(PathValue),
	}
}

func (PathLinkValue) DeepRemove(_ ValueRemoveContext, _ bool) {
	// NO-OP
}

func (v PathLinkValue) ByteSize() uint32 {
	return mustStorableSize(v)
}

func (v PathLinkValue) StoredValue(_ atree.SlabStorage) (atree.Value, error) {
	return v, nil
}

func (v PathLinkValue) ChildStorables() []atree.Storable {
	return []atree.Storable{
		v.TargetPath,
	}
}

// Deprecated: AccountLinkValue
type AccountLinkValue struct{}

var _ Value = AccountLinkValue{}
var _ atree.Value = AccountLinkValue{}
var _ EquatableValue = AccountLinkValue{}
var _ LinkValue = AccountLinkValue{}

func (AccountLinkValue) IsValue() {}

func (AccountLinkValue) isLinkValue() {}

func (v AccountLinkValue) Accept(context ValueVisitContext, visitor Visitor, locationRange LocationRange) {
	panic(errors.NewUnreachableError())
}

func (AccountLinkValue) Walk(_ ValueWalkContext, _ func(Value), _ LocationRange) {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) StaticType(context ValueStaticTypeContext) StaticType {
	// When iterating over public/private paths,
	// the values at these paths are AccountLinkValues,
	// placed there by the `linkAccount` function.
	//
	// These are loaded as links, however,
	// for the purposes of checking their type,
	// we treat them as capabilities
	return NewCapabilityStaticType(
		context,
		NewReferenceStaticType(
			context,
			FullyEntitledAccountAccess,
			PrimitiveStaticTypeAccount,
		),
	)
}

func (AccountLinkValue) IsImportable(_ ValueImportableContext, _ LocationRange) bool {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) String() string {
	return "AccountLink()"
}

func (v AccountLinkValue) RecursiveString(_ SeenReferences) string {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) MeteredString(_ ValueStringContext, _ SeenReferences, _ LocationRange) string {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) ConformsToStaticType(
	_ ValueStaticTypeConformanceContext,
	_ LocationRange,
	_ TypeConformanceResults,
) bool {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) Equal(_ ValueComparisonContext, _ LocationRange, other Value) bool {
	_, ok := other.(AccountLinkValue)
	return ok
}

func (AccountLinkValue) IsStorable() bool {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) Storable(storage atree.SlabStorage, address atree.Address, maxInlineSize uint64) (atree.Storable, error) {
	return values.MaybeLargeImmutableStorable(v, storage, address, maxInlineSize)
}

func (AccountLinkValue) NeedsStoreTo(_ atree.Address) bool {
	panic(errors.NewUnreachableError())
}

func (AccountLinkValue) IsResourceKinded(context ValueStaticTypeContext) bool {
	panic(errors.NewUnreachableError())
}

func (v AccountLinkValue) Transfer(
	context ValueTransferContext,
	_ LocationRange,
	_ atree.Address,
	remove bool,
	storable atree.Storable,
	_ map[atree.ValueID]struct{},
	_ bool,
) Value {
	if remove {
		RemoveReferencedSlab(context, storable)
	}
	return v
}

func (AccountLinkValue) Clone(_ ValueCloneContext) Value {
	return AccountLinkValue{}
}

func (AccountLinkValue) DeepRemove(_ ValueRemoveContext, _ bool) {
	// NO-OP
}

func (v AccountLinkValue) ByteSize() uint32 {
	return mustStorableSize(v)
}

func (v AccountLinkValue) StoredValue(_ atree.SlabStorage) (atree.Value, error) {
	return v, nil
}

func (v AccountLinkValue) ChildStorables() []atree.Storable {
	return nil
}

// NOTE: NEVER change, only add/increment; ensure uint64
const (
	// encodedPathLinkValueTargetPathFieldKey uint64 = 0
	// encodedPathLinkValueTypeFieldKey       uint64 = 1

	// !!! *WARNING* !!!
	//
	// encodedPathLinkValueLength MUST be updated when new element is added.
	// It is used to verify encoded link length during decoding.
	encodedPathLinkValueLength = 2
)

// Encode encodes PathLinkValue as
//
//	cbor.Tag{
//				Number: CBORTagPathLinkValue,
//				Content: []any{
//					encodedPathLinkValueTargetPathFieldKey: PathValue(v.TargetPath),
//					encodedPathLinkValueTypeFieldKey:       StaticType(v.Type),
//				},
//	}
func (v PathLinkValue) Encode(e *atree.Encoder) error {
	// Encode tag number and array head
	err := e.CBOR.EncodeRawBytes([]byte{
		// tag number
		0xd8, values.CBORTagPathLinkValue, //nolint:staticcheck
		// array, 2 items follow
		0x82,
	})
	if err != nil {
		return err
	}
	// Encode path at array index encodedPathLinkValueTargetPathFieldKey
	err = v.TargetPath.Encode(e)
	if err != nil {
		return err
	}
	// Encode type at array index encodedPathLinkValueTypeFieldKey
	return v.Type.Encode(e.CBOR)
}

// cborAccountLinkValue represents the CBOR value:
//
//	cbor.Tag{
//		Number: CBORTagAccountLinkValue,
//		Content: nil
//	}
var cborAccountLinkValue = []byte{
	// tag
	0xd8, values.CBORTagAccountLinkValue, //nolint:staticcheck
	// null
	0xf6,
}

// Encode writes a value of type AccountValue to the encoder
func (AccountLinkValue) Encode(e *atree.Encoder) error {
	return e.CBOR.EncodeRawBytes(cborAccountLinkValue)
}
