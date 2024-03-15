package pgxgeom_test

import (
	"context"
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxtest"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
	"github.com/twpayne/go-geom/encoding/wkt"

	pgxgeom "github.com/twpayne/pgx-geom"
)

var defaultConnTestRunner pgxtest.ConnTestRunner

func init() {
	defaultConnTestRunner = pgxtest.DefaultConnTestRunner()
	defaultConnTestRunner.AfterConnect = func(ctx context.Context, tb testing.TB, conn *pgx.Conn) {
		tb.Helper()
		_, err := conn.Exec(ctx, "create extension if not exists postgis")
		assert.NoError(tb, err)
		assert.NoError(tb, pgxgeom.Register(ctx, conn))
	}
}

func TestCodecDecodeValue(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, tb testing.TB, conn *pgx.Conn) {
		tb.Helper()
		for _, format := range []int16{
			pgx.BinaryFormatCode,
			pgx.TextFormatCode,
		} {
			tb.(*testing.T).Run(strconv.Itoa(int(format)), func(t *testing.T) {
				original := mustNewGeomFromWKT(t, "POINT(1 2)", 4326)
				rows, err := conn.Query(ctx, "select $1::geometry", pgx.QueryResultFormats{format}, original)
				assert.NoError(t, err)

				for rows.Next() {
					values, err := rows.Values()
					assert.NoError(t, err)

					assert.Equal(t, 1, len(values))
					v0, ok := values[0].(geom.T)
					assert.True(t, ok)
					assert.Equal(t, mustEWKB(t, original), mustEWKB(t, v0))
				}

				assert.NoError(t, rows.Err())
			})
		}
	})
}

func TestCodecDecodeNullValue(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, tb testing.TB, conn *pgx.Conn) {
		tb.Helper()
		rows, err := conn.Query(ctx, "select $1::geometry", nil)
		assert.NoError(tb, err)

		for rows.Next() {
			values, err := rows.Values()
			assert.NoError(tb, err)
			assert.Equal(tb, []any{nil}, values)
		}

		assert.NoError(tb, rows.Err())
	})
}

func TestCodecScanValue(t *testing.T) {
	defaultConnTestRunner.RunTest(context.Background(), t, func(ctx context.Context, tb testing.TB, conn *pgx.Conn) {
		tb.Helper()
		for _, format := range []int16{
			pgx.BinaryFormatCode,
			pgx.TextFormatCode,
		} {
			tb.(*testing.T).Run(strconv.Itoa(int(format)), func(t *testing.T) {
				var geom geom.T
				err := conn.QueryRow(ctx, "select ST_SetSRID('POINT(1 2)'::geometry, 4326)", pgx.QueryResultFormats{format}).Scan(&geom)
				assert.NoError(t, err)
				// assert.Equal(t, mustNewGeomFromWKT(t, "POINT(1 2)", 4326).SetSRID(4326).ToEWKBWithSRID(), geom.ToEWKBWithSRID())
			})
		}
	})
}

func mustEWKB(tb testing.TB, g geom.T) []byte {
	tb.Helper()
	data, err := ewkb.Marshal(g, binary.LittleEndian)
	assert.NoError(tb, err)
	return data
}

func mustNewGeomFromWKT(tb testing.TB, s string, srid int) geom.T {
	tb.Helper()
	g, err := wkt.Unmarshal(s)
	assert.NoError(tb, err)
	g, err = geom.SetSRID(g, srid)
	assert.NoError(tb, err)
	return g
}
