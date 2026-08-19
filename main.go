package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	customblocks "github.com/df-mc/dragonfly/plugins/blocks"
	consol "github.com/df-mc/dragonfly/plugins/console"
	customitems "github.com/df-mc/dragonfly/plugins/items"
	"github.com/df-mc/dragonfly/plugins/main/commands"
	"github.com/df-mc/dragonfly/plugins/main/data"
	maindb "github.com/df-mc/dragonfly/plugins/main/db"
	"github.com/df-mc/dragonfly/plugins/main/events"
	"github.com/df-mc/dragonfly/plugins/main/floatingtext"
	"github.com/df-mc/dragonfly/plugins/main/npc"
	"github.com/df-mc/dragonfly/plugins/main/pvpmines"

	pluginserver "github.com/df-mc/dragonfly/plugins/main/server"
	worldhandler "github.com/df-mc/dragonfly/plugins/main/worldHandler"
	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/cmd"
	"github.com/df-mc/dragonfly/server/player/chat"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/biome"
	"github.com/df-mc/dragonfly/server/world/generator"
	"github.com/pelletier/go-toml"
	_ "modernc.org/sqlite"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	chat.Global.Subscribe(chat.StdoutSubscriber{})
	customblocks.Register()
	customitems.Register()
	conf, err := readConfig(slog.Default())
	if err != nil {
		panic(err)
	}

	conf.Entities = npc.Registry()
	srv := conf.New()
	database, err := openDatabase()
	if err != nil {
		panic(err)
	}
	defer database.Close()

	pluginserver.ServerInfoInstance.DB = database
	if err := maindb.InitDB(); err != nil {
		panic(fmt.Errorf("initialize plugin database: %w", err))
	}

	enablePlugins(srv, slog.Default())
	// The server owns the default dimensions, but plugin worlds such as the
	// PvP mine and personal mines are created separately. Ensure those worlds
	// are also closed and flushed when the process exits after Ctrl+C.
	defer worldhandler.GlobalWorldHandler.CloseCustomWorlds()
	events.StartPlayerStatsTitles(srv)
	srv.CloseOnProgramEnd()

	srv.Listen()
	for p := range srv.Accept() {
		// Attach the handler before running any join logic. Player values yielded
		// by Accept are only valid inside this loop body.
		p.Handle(events.NewPlayerEventHandler())

		// Create the player's row before commands/forms try to read their stats.
		// A database failure is logged, but should not disconnect the player.
		if err := maindb.InitPlayer(p); err != nil {
			slog.Default().Error("initialize player database row", "player", p.Name(), "error", err)
		}
		events.HandleJoin(p)
	}
}

func openDatabase() (*sql.DB, error) {
	if err := os.MkdirAll("db", 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	database, err := sql.Open("sqlite", "db/carbon_prisons.sqlite")
	if err != nil {
		return nil, fmt.Errorf("open plugin database: %w", err)
	}
	// SQLite permits one writer at a time. Serializing access through one
	// connection avoids lock contention between player joins, sells, and upgrades.
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect to plugin database: %w", err)
	}
	return database, nil
}

// enablePlugins performs all plugin setup that Dragonfly does not do
// automatically. Packages under plugins/ are ordinary Go packages; they do
// not become active just by being present in the repository.
func enablePlugins(srv *server.Server, log *slog.Logger) {
	pluginserver.ServerInfoInstance.Server = srv

	worldhandler.GlobalWorldHandler.RegisterDefaultWorld(srv, "spawn")
	npc.SpawnInWorld(srv.World())
	floatingtext.SpawnInWorld(srv.World())
	srv.World().Handle(events.NewWorldEventHandler())
	if pvpMine, ok := worldhandler.GlobalWorldHandler.CreateSimpleWorld(
		worldhandler.PvpMineWorldPath,
		world.Config{
			Log:       log,
			Generator: generator.NewFlat(biome.Plains{}, []world.Block{block.Stone{}}),
		},
		worldhandler.PvpMineWorldID,
	); ok {
		pvpMine.World.Handle(events.NewWorldEventHandler())
		pvpMine.World.SetSpawn(cube.PosFromVec3(data.PVP_MINE_LOCATION.Pos))
		pvpmines.Refill(pvpMine.World)
		pvpmines.StartHourlyRefill(pvpMine.World)
	}

	cmd.Register(cmd.New("spawn", "Teleport to spawn.", nil, commands.SpawnCommand{}))
	cmd.Register(cmd.New("adminpanel", "Open the admin panel.", nil, commands.AdminPanelCommand{}))

	consol.Enable(srv, log)
	log.Info("Plugins enabled", "spawn_world", worldhandler.GlobalWorldHandler.SpawnWorldID)
}

// readConfig reads the configuration from the config.toml file, or creates the
// file if it does not yet exist.
func readConfig(log *slog.Logger) (server.Config, error) {
	c := server.DefaultConfig()
	var zero server.Config
	if _, err := os.Stat("config.toml"); os.IsNotExist(err) {
		data, err := toml.Marshal(c)
		if err != nil {
			return zero, fmt.Errorf("encode default config: %v", err)
		}
		if err := os.WriteFile("config.toml", data, 0644); err != nil {
			return zero, fmt.Errorf("create default config: %v", err)
		}
		return c.Config(log)
	}
	data, err := os.ReadFile("config.toml")
	if err != nil {
		return zero, fmt.Errorf("read config: %v", err)
	}
	if err := toml.Unmarshal(data, &c); err != nil {
		return zero, fmt.Errorf("decode config: %v", err)
	}
	return c.Config(log)
}
