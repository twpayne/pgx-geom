# pgx-geom

[![PkgGoDev](https://pkg.go.dev/badge/github.com/twpayne/pgx-geom)](https://pkg.go.dev/github.com/twpayne/pgx-geom)

Package pgx-geom provides [PostGIS](https://postgis.net/) support for
[`github.com/jackc/pgx/v5`](https://pkg.go.dev/github.com/jackc/pgx/v5) via
[`github.com/twpayne/go-geom`](https://pkg.go.dev/github.com/twpayne/go-geom).

## Usage

### Single connection

```go
import (
    // ...

    "github.com/jackc/pgx/v5"
    "github.com/twpayne/go-geom"
    pgxgeom "github.com/twpayne/pgx-geom"
)

// ...

    connectionStr := os.Getenv("DATABASE_URL")
    conn, err := pgx.Connect(context.Background(), connectionStr)
    if err != nil {
        return err
    }
    if err := pgxgeom.Register(ctx, conn); err != nil {
        return err
    }
```

### Connection pool

```go
import (
    // ...

    "github.com/jackc/pgx/v5/pgxpool"
)

// ...

    config, err := pgxpool.ParseConfig(connectionStr)
    if err != nil {
        return err
    }
    config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
        if err := pgxgeom.Register(ctx, conn); err != nil {
            return err
        }
        return nil
    }

    pool, err := pgxpool.NewWithConfig(context.Background(), config)
    if err != nil {
        return err
    }
```

## sqlc

See [the sqlc documentation](https://docs.sqlc.dev/en/latest/reference/datatypes.html#using-github-com-twpayne-go-geom).

## License

MIT