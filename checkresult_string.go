// Code generated by "stringer -type CheckResult"; DO NOT EDIT.

package sardine

import "strconv"

const _CheckResult_name = "CheckOKCheckFailedCheckWarning"

var _CheckResult_index = [...]uint8{0, 7, 18, 30}

func (i CheckResult) String() string {
	if i < 0 || i >= CheckResult(len(_CheckResult_index)-1) {
		return "CheckResult(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _CheckResult_name[_CheckResult_index[i]:_CheckResult_index[i+1]]
}
