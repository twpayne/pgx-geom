package pgxgeom

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

// A codec implements [github.com/jackc/pgx/v5/pgtype.Codec] for
// [github.com/twpayne/go-geom.T] types.
type codec struct{}

// A binaryEncodePlan implements [github.com/jackc/pgx/v5/pgtype.EncodePlan] for
// [github.com/twpayne/go-geom.T] types in binary format.
type binaryEncodePlan struct{}

// A textEncodePlan implements [github.com/jackc/pgx/v5/pgtype.EncodePlan] for
// [github.com/twpayne/go-geom.T] types in text format.
type textEncodePlan struct{}

// A binaryScanPlan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan] for
// [github.com/twpayne/go-geom.T] types in binary format.
type binaryScanPlan struct{}

// A textScanPlan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan] for
// [github.com/twpayne/go-geom.T] types in text format.
type textScanPlan struct{}

// nativeEndian is the host's native byte order. We have to determine this with
// a runtime test in init() because binary.NativeEndian is a separate value to
// binary.LittleEndian and binary.BigEndian.
var nativeEndian binary.ByteOrder

func init() {
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	switch binary.NativeEndian.Uint64(data) {
	case binary.LittleEndian.Uint64(data):
		nativeEndian = binary.LittleEndian
	case binary.BigEndian.Uint64(data):
		nativeEndian = binary.BigEndian
	default:
		panic("unsupported byte order")
	}
}

// FormatSupported implements
// [github.com/jackc/pgx/v5/pgtype.Codec.FormatSupported].
func (c codec) FormatSupported(format int16) bool {
	switch format {
	case pgtype.BinaryFormatCode:
		return true
	case pgtype.TextFormatCode:
		return true
	default:
		return false
	}
}

// PreferredFormat implements
// [github.com/jackc/pgx/v5/pgtype.Codec.PreferredFormat].
func (c codec) PreferredFormat() int16 {
	return pgtype.BinaryFormatCode
}

// PlanEncode implements [github.com/jackc/pgx/v5/pgtype.Codec.PlanEncode].
func (c codec) PlanEncode(m *pgtype.Map, old uint32, format int16, value any) pgtype.EncodePlan {
	if _, ok := value.(geom.T); !ok {
		return nil
	}
	switch format {
	case pgtype.BinaryFormatCode:
		return binaryEncodePlan{}
	case pgtype.TextFormatCode:
		return textEncodePlan{}
	default:
		return nil
	}
}

// PlanScan implements [github.com/jackc/pgx/v5/pgtype.Codec.PlanScan].
func (c codec) PlanScan(m *pgtype.Map, old uint32, format int16, target any) pgtype.ScanPlan {
	if _, ok := target.(*geom.T); !ok {
		return nil
	}
	switch format {
	case pgx.BinaryFormatCode:
		return &binaryScanPlan{}
	case pgx.TextFormatCode:
		return &textScanPlan{}
	default:
		return nil
	}
}

// DecodeDatabaseSQLValue implements
// [github.com/jackc/pgx/v5/pgtype.Codec.DecodeDatabaseSQLValue].
func (c codec) DecodeDatabaseSQLValue(m *pgtype.Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	return nil, errors.ErrUnsupported
}

// DecodeValue implements [github.com/jackc/pgx/v5/pgtype.Codec.DecodeValue].
func (c codec) DecodeValue(m *pgtype.Map, oid uint32, format int16, src []byte) (any, error) {
	switch format {
	case pgtype.TextFormatCode:
		var err error
		src, err = hex.DecodeString(string(src))
		if err != nil {
			return nil, err
		}
		fallthrough
	case pgtype.BinaryFormatCode:
		return ewkb.Unmarshal(src)
	default:
		return nil, errors.ErrUnsupported
	}
}

// Encode implements [github.com/jackc/pgx/v5/pgtype.EncodePlan.Encode].
func (p binaryEncodePlan) Encode(value any, buf []byte) (newBuf []byte, err error) {
	g, ok := value.(geom.T)
	if !ok {
		return buf, errors.ErrUnsupported
	}
	data, err := ewkb.Marshal(g, nativeEndian)
	if err != nil {
		return buf, err
	}
	return append(buf, data...), nil
}

// Encode implements [github.com/jackc/pgx/v5/pgtype.EncodePlan.Encode].
func (p textEncodePlan) Encode(value any, buf []byte) (newBuf []byte, err error) {
	g, ok := value.(geom.T)
	if !ok {
		return buf, errors.ErrUnsupported
	}
	data, err := ewkb.Marshal(g, nativeEndian)
	if err != nil {
		return buf, err
	}
	return append(buf, []byte(hex.EncodeToString(data))...), nil
}

// Scan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan.Scan].
func (p binaryScanPlan) Scan(src []byte, target any) error {
	pg, ok := target.(*geom.T)
	if !ok {
		return errors.ErrUnsupported
	}
	if len(src) == 0 {
		*pg = nil
		return nil
	}
	g, err := ewkb.Unmarshal(src)
	if err != nil {
		return err
	}
	*pg = g
	return nil
}

// Scan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan.Scan].
func (p textScanPlan) Scan(src []byte, target any) error {
	pg, ok := target.(*geom.T)
	if !ok {
		return errors.ErrUnsupported
	}
	if len(src) == 0 {
		*pg = nil
		return nil
	}
	var err error
	src, err = hex.DecodeString(string(src))
	if err != nil {
		return err
	}
	g, err := ewkb.Unmarshal(src)
	if err != nil {
		return err
	}
	*pg = g
	return nil
}

// Register registers a codec for [github.com/twpayne/go-geom.T] types on conn.
func Register(ctx context.Context, conn *pgx.Conn) error {
	var oid uint32
	err := conn.QueryRow(ctx, "select 'geometry'::text::regtype::oid").Scan(&oid)
	if err != nil {
		return err
	}

	conn.TypeMap().RegisterType(&pgtype.Type{
		Codec: codec{},
		Name:  "geometry",
		OID:   oid,
	})

	return nil
}
