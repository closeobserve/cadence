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

package interpreter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence/common"
	"github.com/onflow/cadence/interpreter"
	"github.com/onflow/cadence/sema"
	. "github.com/onflow/cadence/test_utils/interpreter_utils"
	. "github.com/onflow/cadence/test_utils/sema_utils"
)

func TestInterpretResultVariable(t *testing.T) {

	t.Parallel()

	t.Run("resource type, resource value", func(t *testing.T) {
		t.Parallel()

		inter := parseCheckAndInterpret(t, `
            access(all) resource R {
                access(all) let id: UInt8
                init() {
                    self.id = 1
                }
            }

            access(all) fun main(): @R  {
                post {
                    result.id == 1: "invalid id"
                }
                return <- create R()
            }`,
		)

		result, err := inter.Invoke("main")
		require.NoError(t, err)

		require.IsType(t, &interpreter.CompositeValue{}, result)
		resource := result.(*interpreter.CompositeValue)
		assert.Equal(t, common.CompositeKindResource, resource.Kind)
		AssertValuesEqual(
			t,
			inter,
			interpreter.UInt8Value(1),
			resource.GetField(inter, interpreter.EmptyLocationRange, "id"),
		)
	})

	t.Run("optional resource type, resource value", func(t *testing.T) {
		t.Parallel()

		inter := parseCheckAndInterpret(t, `
            access(all) resource R {
                access(all) let id: UInt8
                init() {
                    self.id = 1
                }
            }

            access(all) fun main(): @R?  {
                post {
                    result!.id == 1: "invalid id"
                }
                return <- create R()
            }`,
		)

		result, err := inter.Invoke("main")
		require.NoError(t, err)

		require.IsType(t, &interpreter.SomeValue{}, result)
		someValue := result.(*interpreter.SomeValue)

		innerValue := someValue.InnerValue(inter, interpreter.EmptyLocationRange)
		require.IsType(t, &interpreter.CompositeValue{}, innerValue)

		resource := innerValue.(*interpreter.CompositeValue)
		assert.Equal(t, common.CompositeKindResource, resource.Kind)
		AssertValuesEqual(
			t,
			inter,
			interpreter.UInt8Value(1),
			resource.GetField(inter, interpreter.EmptyLocationRange, "id"),
		)
	})

	t.Run("optional resource type, nil value", func(t *testing.T) {
		t.Parallel()

		inter := parseCheckAndInterpret(t, `
            access(all) resource R {
                access(all) let id: UInt8
                init() {
                    self.id = 1
                }
            }

            access(all) fun main(): @R?  {
                post {
                    result == nil: "invalid result"
                }
                return nil
            }`,
		)

		result, err := inter.Invoke("main")
		require.NoError(t, err)
		require.Equal(t, interpreter.NilValue{}, result)
	})

	t.Run("any resource type, optional value", func(t *testing.T) {
		t.Parallel()

		inter := parseCheckAndInterpret(t, `
            access(all) resource R {
                access(all) let id: UInt8
                init() {
                    self.id = 1
                }
            }

            access(all) fun main(): @AnyResource  {
                post {
                    result != nil: "invalid value"
                }

                var value: @R? <- create R()
                return <- value
            }`,
		)

		result, err := inter.Invoke("main")
		require.NoError(t, err)

		require.IsType(t, &interpreter.SomeValue{}, result)
		someValue := result.(*interpreter.SomeValue)

		innerValue := someValue.InnerValue(inter, interpreter.EmptyLocationRange)
		require.IsType(t, &interpreter.CompositeValue{}, innerValue)

		resource := innerValue.(*interpreter.CompositeValue)
		assert.Equal(t, common.CompositeKindResource, resource.Kind)
		AssertValuesEqual(
			t,
			inter,
			interpreter.UInt8Value(1),
			resource.GetField(inter, interpreter.EmptyLocationRange, "id"),
		)
	})

	t.Run("reference invalidation, optional type", func(t *testing.T) {
		t.Parallel()

		var checkerErrors []error

		inter, err := parseCheckAndInterpretWithOptions(t, `
            access(all) resource R {
                access(all) var id: UInt8
				access(all) fun setID(_ id: UInt8) {
					self.id = id
				}
                init() {
                    self.id = 1
                }
            }

            var ref: &R? = nil

            access(all) fun main(): @R? {
                var r <- createAndStoreRef()
                if var r2 <- r {
                    r2.setID(2)
                    return <- r2
                }

                return nil
            }

            access(all) fun createAndStoreRef(): @R? {
                post {
                    storeRef(result)
                }
                return <- create R()
            }

            access(all) fun storeRef(_ r: &R?): Bool {
                ref = r
                return r != nil
            }

            access(all) fun getID(): UInt8 {
                return ref!.id
            }`,
			ParseCheckAndInterpretOptions{
				HandleCheckerError: func(err error) {
					checkerErrors = append(checkerErrors, err)
				},
			},
		)
		require.NoError(t, err)
		require.Len(t, checkerErrors, 1)
		checkerError := RequireCheckerErrors(t, checkerErrors[0], 1)
		require.IsType(t, &sema.PurityError{}, checkerError[0])

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		_, err = inter.Invoke("getID")
		require.ErrorAs(t, err, &interpreter.InvalidatedResourceReferenceError{})
	})

	t.Run("reference invalidation, non optional", func(t *testing.T) {
		t.Parallel()

		var checkerErrors []error

		inter, err := parseCheckAndInterpretWithOptions(t, `
            access(all) resource R {
                access(all) var id: UInt8

				access(all) fun setID(_ id: UInt8) {
					self.id = id
				}

                init() {
                    self.id = 1
                }
            }

            var ref: &R? = nil

            access(all) fun main(): @R {
                var r <- createAndStoreRef()
                r.setID(2)
                return <- r
            }

            access(all) fun createAndStoreRef(): @R {
                post {
                    storeRef(result)
                }
                return <- create R()
            }

            access(all) fun storeRef(_ r: &R): Bool {
                ref = r
                return r != nil
            }

            access(all) fun getID(): UInt8 {
                return ref!.id
            }`,
			ParseCheckAndInterpretOptions{
				HandleCheckerError: func(err error) {
					checkerErrors = append(checkerErrors, err)
				},
			},
		)
		require.NoError(t, err)
		require.Len(t, checkerErrors, 1)
		checkerError := RequireCheckerErrors(t, checkerErrors[0], 1)
		require.IsType(t, &sema.PurityError{}, checkerError[0])

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		_, err = inter.Invoke("getID")
		require.ErrorAs(t, err, &interpreter.InvalidatedResourceReferenceError{})
	})
}

func TestInterpretFunctionSubtyping(t *testing.T) {

	t.Parallel()

	inter := parseCheckAndInterpret(t, `
        struct T {
            var bar: UInt8
            init() {
                self.bar = 4
            }
        }

        access(all) fun foo(): T {
            return T()
        }

        access(all) fun main(): UInt8?  {
            var f: (fun(): T?) = foo
            return f()?.bar
        }`,
	)

	result, err := inter.Invoke("main")
	require.NoError(t, err)

	AssertValuesEqual(
		t,
		inter,
		interpreter.NewUnmeteredSomeValueNonCopying(interpreter.UInt8Value(4)),
		result,
	)
}

func TestInterpretGenericFunctionSubtyping(t *testing.T) {

	t.Parallel()

	t.Run("storage.load as non-generic function", func(t *testing.T) {
		t.Parallel()

		address := interpreter.NewUnmeteredAddressValueFromBytes([]byte{42})

		inter, _ := testAccount(t, address, true, nil, `
            fun test() {
                var boxedFunc: AnyStruct = account.storage.load
                var unboxedFunc = boxedFunc as! fun(StoragePath):AnyStruct
            }
            `,
			sema.Config{},
		)

		_, err := inter.Invoke("test")
		require.Error(t, err)

		var typeErr interpreter.ForceCastTypeMismatchError
		require.ErrorAs(t, err, &typeErr)
	})

	t.Run("storage.copy as non-generic function", func(t *testing.T) {
		t.Parallel()

		address := interpreter.NewUnmeteredAddressValueFromBytes([]byte{42})

		inter, _ := testAccount(t, address, true, nil, `
            fun test() {
                var boxedFunc: AnyStruct = account.storage.copy
                var unboxedFunc = boxedFunc as! fun(StoragePath):AnyStruct
            }
            `,
			sema.Config{},
		)

		_, err := inter.Invoke("test")
		require.Error(t, err)

		var typeErr interpreter.ForceCastTypeMismatchError
		require.ErrorAs(t, err, &typeErr)
	})
}
