// Code generated by "stringer -type=opCode -trimprefix=op"; DO NOT EDIT.

package expr

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[opNop-0]
	_ = x[opRet-1]
	_ = x[opDerefAny-2]
	_ = x[opDerefBool-3]
	_ = x[opDerefFloat-4]
	_ = x[opDerefInt-5]
	_ = x[opDerefStr-6]
	_ = x[opLdv-7]
	_ = x[opNotBool-8]
	_ = x[opNotInt-9]
	_ = x[opEqAny-10]
	_ = x[opEqBool-11]
	_ = x[opEqFloat-12]
	_ = x[opEqInt-13]
	_ = x[opEqRat-14]
	_ = x[opEqStr-15]
	_ = x[opNeAny-16]
	_ = x[opNeBool-17]
	_ = x[opNeFloat-18]
	_ = x[opNeInt-19]
	_ = x[opNeRat-20]
	_ = x[opNeStr-21]
	_ = x[opLtAny-22]
	_ = x[opLtFloat-23]
	_ = x[opLtInt-24]
	_ = x[opLtRat-25]
	_ = x[opLtStr-26]
	_ = x[opLeAny-27]
	_ = x[opLeFloat-28]
	_ = x[opLeInt-29]
	_ = x[opLeRat-30]
	_ = x[opLeStr-31]
	_ = x[opGeAny-32]
	_ = x[opGeFloat-33]
	_ = x[opGeInt-34]
	_ = x[opGeRat-35]
	_ = x[opGeStr-36]
	_ = x[opGtAny-37]
	_ = x[opGtFloat-38]
	_ = x[opGtInt-39]
	_ = x[opGtRat-40]
	_ = x[opGtStr-41]
	_ = x[opAndBool-42]
	_ = x[opAndInt-43]
	_ = x[opAndRat-44]
	_ = x[opOrBool-45]
	_ = x[opOrInt-46]
	_ = x[opOrRat-47]
	_ = x[opAddFloat-48]
	_ = x[opAddInt-49]
	_ = x[opAddRat-50]
	_ = x[opAddStr-51]
	_ = x[opSubFloat-52]
	_ = x[opSubInt-53]
	_ = x[opSubRat-54]
	_ = x[opMulFloat-55]
	_ = x[opMulInt-56]
	_ = x[opMulRat-57]
	_ = x[opDivFloat-58]
	_ = x[opDivInt-59]
	_ = x[opDivRat-60]
	_ = x[opLdmAnyAny-61]
	_ = x[opLdmStrMapAny-62]
	_ = x[opConvFloat64ToInt64-63]
	_ = x[opConvFloat64ToRat-64]
	_ = x[opConvInt64ToFloat64-65]
	_ = x[opConvInt64ToRat-66]
	_ = x[opConvRatToFloat64-67]
	_ = x[opConvRatToInt64-68]
	_ = x[opLdc-69]
	_ = x[opPackTuple-70]
	_ = x[opBrt-71]
}

const _opCode_name = "NopRetDerefAnyDerefBoolDerefFloatDerefIntDerefStrLdvNotBoolNotIntEqAnyEqBoolEqFloatEqIntEqRatEqStrNeAnyNeBoolNeFloatNeIntNeRatNeStrLtAnyLtFloatLtIntLtRatLtStrLeAnyLeFloatLeIntLeRatLeStrGeAnyGeFloatGeIntGeRatGeStrGtAnyGtFloatGtIntGtRatGtStrAndBoolAndIntAndRatOrBoolOrIntOrRatAddFloatAddIntAddRatAddStrSubFloatSubIntSubRatMulFloatMulIntMulRatDivFloatDivIntDivRatLdmAnyAnyLdmStrMapAnyConvFloat64ToInt64ConvFloat64ToRatConvInt64ToFloat64ConvInt64ToRatConvRatToFloat64ConvRatToInt64LdcPackTupleBrt"

var _opCode_index = [...]uint16{0, 3, 6, 14, 23, 33, 41, 49, 52, 59, 65, 70, 76, 83, 88, 93, 98, 103, 109, 116, 121, 126, 131, 136, 143, 148, 153, 158, 163, 170, 175, 180, 185, 190, 197, 202, 207, 212, 217, 224, 229, 234, 239, 246, 252, 258, 264, 269, 274, 282, 288, 294, 300, 308, 314, 320, 328, 334, 340, 348, 354, 360, 369, 381, 399, 415, 433, 447, 463, 477, 480, 489, 492}

func (i opCode) String() string {
	if i >= opCode(len(_opCode_index)-1) {
		return "opCode(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _opCode_name[_opCode_index[i]:_opCode_index[i+1]]
}
