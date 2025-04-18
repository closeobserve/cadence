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
	"github.com/onflow/atree"

	"github.com/onflow/cadence/common"
	"github.com/onflow/cadence/values"
)

type StringAtreeValue string

var _ atree.Value = StringAtreeValue("")
var _ atree.Storable = StringAtreeValue("")
var _ atree.ComparableStorable = StringAtreeValue("")

func (v StringAtreeValue) Storable(
	storage atree.SlabStorage,
	address atree.Address,
	maxInlineSize uint64,
) (
	atree.Storable,
	error,
) {
	return values.MaybeLargeImmutableStorable(v, storage, address, maxInlineSize)
}

func NewStringAtreeValue(gauge common.MemoryGauge, s string) StringAtreeValue {
	common.UseMemory(gauge, common.NewRawStringMemoryUsage(len(s)))
	return StringAtreeValue(s)
}

func (v StringAtreeValue) ByteSize() uint32 {
	return values.GetBytesCBORSize([]byte(v))
}

func (v StringAtreeValue) StoredValue(_ atree.SlabStorage) (atree.Value, error) {
	return v, nil
}

func (StringAtreeValue) ChildStorables() []atree.Storable {
	return nil
}

// Equal returns true if the given storable is equal to this StringAtreeValue.
func (v StringAtreeValue) Equal(other atree.Storable) bool {
	v1, ok := other.(StringAtreeValue)
	return ok && v == v1
}

// Less returns true if the given storable is less than StringAtreeValue.
func (v StringAtreeValue) Less(other atree.Storable) bool {
	return v < other.(StringAtreeValue)
}

// ID returns a unique identifier.
func (v StringAtreeValue) ID() string {
	return string(v)
}

func (v StringAtreeValue) Copy() atree.Storable {
	return v
}

func StringAtreeValueHashInput(v atree.Value, _ []byte) ([]byte, error) {
	return []byte(v.(StringAtreeValue)), nil
}

func StringAtreeValueComparator(storage atree.SlabStorage, value atree.Value, otherStorable atree.Storable) (bool, error) {
	otherValue, err := otherStorable.StoredValue(storage)
	if err != nil {
		return false, err
	}
	result := value.(StringAtreeValue) == otherValue.(StringAtreeValue)
	return result, nil
}
