package buf2struct

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Timestamp converts a *timestamppb.Timestamp to *time.Time
func Timestamp(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// TimestampValue converts a *timestamppb.Timestamp to time.Time (not pointer)
func TimestampValue(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func Int32PtrToIntPtr(i32 *int32) *int {
	if i32 == nil {
		return nil
	}
	i := int(*i32)
	return &i
}
