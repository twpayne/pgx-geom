package pgxgeom

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

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

// GeomScanner enables PostGIS geometry/geography values to be scanned into
// arbitrary Go types. For more context, see section "Extending Existing
// PostgreSQL Type Support" of the README for jackc/pgx/v5/pgtype.
type GeomScanner interface {
	ScanGeom(v geom.T) error
}

// GeomValuer enables PostGIS geometry/geography values to be marshaled from
// arbitrary Go types. For more context, see section "Extending Existing
// PostgreSQL Type Support" of the README for jackc/pgx/v5/pgtype.
type GeomValuer interface {
	GeomValue() (geom.T, error)
}

// unexpectedTypeError indicates that a PostGIS value did not meet the type
// constraints to be scanned into a particular Go value. For example, this
// occurs when attempting to scan a `geometry(point)` into a `*geom.Polygon`.
type unexpectedTypeError struct {
	Got  any
	Want any
}

func (e unexpectedTypeError) Error() string {
	return fmt.Sprintf("pgxgeom: got %T, want %T", e.Got, e.Want)
}

// unsupportedTypeError indicates that a given Go value could not be converted to
// a GeomScanner/GeomValuer. For example, this occurs if you attempt to scan
// into a `*bool`.
type unsupportedTypeError struct {
	Got any
}

func (e unsupportedTypeError) Error() string {
	return fmt.Sprintf("pgxgeom: unsupported type %T", e.Got)
}

// genericGeomValuer can be used to marshal generic geom.T values as well as
// any concrete value like a *geom.Point.
type genericGeomValuer struct {
	value geom.T
}

func (gv genericGeomValuer) GeomValue() (geom.T, error) {
	return gv.value, nil
}

// genericGeomScanner can only be used to scan into generic geom.T values. To
// scan into concrete values like a *geom.Point, a more specific scanner type
// is needed to perform the appropriate error checking.
type genericGeomScanner struct {
	target *geom.T
}

func (sc genericGeomScanner) ScanGeom(v geom.T) error {
	*sc.target = v
	return nil
}

// concreteScanner is used to scan into a specific, concrete geom.T type.
// The type parameter T should be in *non-pointer* form, like `geom.Point`,
// such that `*T` implements `geom.T`.
type concreteScanner[T any] struct {
	target *T
}

func (sc concreteScanner[T]) ScanGeom(v geom.T) error {
	var vv any = v // work around "impossible type assertion" compiler error
	concrete, ok := vv.(*T)
	if !ok {
		return unexpectedTypeError{Got: v, Want: sc.target}
	}
	*sc.target = *concrete
	return nil
}

func getGeomScanner(v any) (GeomScanner, error) {
	switch v := v.(type) {
	case GeomScanner:
		return v, nil
	case *geom.T:
		return genericGeomScanner{v}, nil
	case *geom.Point:
		return concreteScanner[geom.Point]{v}, nil
	case *geom.LineString:
		return concreteScanner[geom.LineString]{v}, nil
	case *geom.Polygon:
		return concreteScanner[geom.Polygon]{v}, nil
	case *geom.MultiPoint:
		return concreteScanner[geom.MultiPoint]{v}, nil
	case *geom.MultiLineString:
		return concreteScanner[geom.MultiLineString]{v}, nil
	case *geom.MultiPolygon:
		return concreteScanner[geom.MultiPolygon]{v}, nil
	case *geom.GeometryCollection:
		return concreteScanner[geom.GeometryCollection]{v}, nil
	default:
		return nil, unsupportedTypeError{v}
	}
}

//nolint:ireturn
func getGeomValuer(v any) (GeomValuer, error) {
	switch v := v.(type) {
	case GeomValuer:
		return v, nil
	case geom.T:
		return genericGeomValuer{v}, nil
	default:
		return nil, unsupportedTypeError{v}
	}
}

// PlanEncode implements [github.com/jackc/pgx/v5/pgtype.Codec.PlanEncode].
func (c codec) PlanEncode(m *pgtype.Map, old uint32, format int16, value any) pgtype.EncodePlan {
	if _, err := getGeomValuer(value); err != nil {
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
	if _, err := getGeomScanner(target); err != nil {
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

func encodeGeomValue(value any) (ewkbBuf []byte, err error) {
	valuer, err := getGeomValuer(value)
	if err != nil {
		return nil, err
	}
	g, err := valuer.GeomValue()
	if err != nil {
		return nil, err
	}
	return ewkb.Marshal(g, nativeEndian)
}

// Encode implements [github.com/jackc/pgx/v5/pgtype.EncodePlan.Encode].
func (p binaryEncodePlan) Encode(value any, buf []byte) (newBuf []byte, err error) {
	data, err := encodeGeomValue(value)
	if err != nil {
		return buf, err
	}
	return append(buf, data...), nil
}

// Encode implements [github.com/jackc/pgx/v5/pgtype.EncodePlan.Encode].
func (p textEncodePlan) Encode(value any, buf []byte) (newBuf []byte, err error) {
	data, err := encodeGeomValue(value)
	if err != nil {
		return buf, err
	}
	return append(buf, []byte(hex.EncodeToString(data))...), nil
}

// Scan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan.Scan].
func (p binaryScanPlan) Scan(src []byte, target any) error {
	scanner, err := getGeomScanner(target)
	if err != nil {
		return err
	}
	if len(src) == 0 {
		return scanner.ScanGeom(nil)
	}
	g, err := ewkb.Unmarshal(src)
	if err != nil {
		return err
	}
	return scanner.ScanGeom(g)
}

// Scan implements [github.com/jackc/pgx/v5/pgtype.ScanPlan.Scan].
func (p textScanPlan) Scan(src []byte, target any) error {
	scanner, err := getGeomScanner(target)
	if err != nil {
		return err
	}
	if len(src) == 0 {
		return scanner.ScanGeom(nil)
	}
	src, err = hex.DecodeString(string(src))
	if err != nil {
		return err
	}
	g, err := ewkb.Unmarshal(src)
	if err != nil {
		return err
	}
	return scanner.ScanGeom(g)
}

// Register registers a codec for [github.com/twpayne/go-geom.T] types on conn.
func Register(ctx context.Context, conn *pgx.Conn) error {
	var geographyOID, geometryOID uint32
	err := conn.QueryRow(ctx, "select 'geography'::text::regtype::oid, 'geometry'::text::regtype::oid").Scan(&geographyOID, &geometryOID)
	if err != nil {
		return err
	}

	conn.TypeMap().RegisterType(&pgtype.Type{
		Codec: codec{},
		Name:  "geography",
		OID:   geographyOID,
	})

	conn.TypeMap().RegisterType(&pgtype.Type{
		Codec: codec{},
		Name:  "geometry",
		OID:   geometryOID,
	})

	return nil
}
