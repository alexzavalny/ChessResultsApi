// Easy Chess Results widget for Scriptable.
// Paste this file into Scriptable and add it as a medium or large widget.

const API_BASE = "https://transpose-junkie-outlet.ngrok-free.dev";
const REFRESH_MINUTES = 30;
const DEFAULT_PARAMETER = "1451269-15";

// Widget parameter format: tournamentId-startNumber (for example, 1451269-15).
const widgetParameter = String(args.widgetParameter || DEFAULT_PARAMETER).trim();
const parameterMatch = widgetParameter.match(/^(\d+)-(\d+)$/);
if (!parameterMatch) {
  throw new Error("Invalid widget parameter. Use tournamentId-startNumber, for example 1451269-15.");
}

const TOURNAMENT_ID = parameterMatch[1];
const START_NUMBER = parameterMatch[2];

const API_URL = `${API_BASE}/api/v1/tournaments/${TOURNAMENT_ID}/players/${START_NUMBER}/results`;
const SOURCE_URL = `https://s2.chess-results.com/tnr${TOURNAMENT_ID}.aspx?lan=1&art=9&snr=${START_NUMBER}`;

const family = config.widgetFamily || "medium";
const maximumRows = family === "large" ? 9 : family === "small" ? 3 : 5;
const opponentWidth = family === "small" ? 74 : family === "extraLarge" ? 250 : 154;
const cache = FileManager.local();
const cachePath = cache.joinPath(
  cache.documentsDirectory(),
  `chess-results-${TOURNAMENT_ID}-${START_NUMBER}.json`
);

let payload;
let isCached = false;

try {
  const request = new Request(API_URL);
  request.timeoutInterval = 15;
  payload = await request.loadJSON();
  cache.writeString(cachePath, JSON.stringify(payload));
} catch (error) {
  if (!cache.fileExists(cachePath)) throw error;
  payload = JSON.parse(cache.readString(cachePath));
  isCached = true;
}

const widget = createWidget(payload.data, isCached);
widget.url = SOURCE_URL;
widget.refreshAfterDate = new Date(
  Date.now() + (isCached ? 10 : REFRESH_MINUTES) * 60 * 1000
);

Script.setWidget(widget);
if (!config.runsInWidget) await widget.presentMedium();
Script.complete();

function createWidget(data, stale) {
  const widget = new ListWidget();
  widget.setPadding(13, 14, 12, 14);

  const gradient = new LinearGradient();
  gradient.colors = [new Color("111827"), new Color("1f2937")];
  gradient.locations = [0, 1];
  widget.backgroundGradient = gradient;

  const header = widget.addStack();
  header.centerAlignContent();

  const identity = header.addStack();
  identity.layoutVertically();

  const name = identity.addText(data.player.name);
  name.font = Font.boldSystemFont(family === "small" ? 15 : 19);
  name.textColor = Color.white();
  name.lineLimit = 1;
  name.minimumScaleFactor = 0.7;

  const subtitle = identity.addText(`#${data.player.rank}  •  ${data.player.points} pts`);
  subtitle.font = Font.semiboldSystemFont(12);
  subtitle.textColor = new Color("9ca3af");

  header.addSpacer();

  const status = header.addText(stale ? "CACHED" : "LIVE");
  status.font = Font.boldSystemFont(10);
  status.textColor = stale ? new Color("fbbf24") : new Color("34d399");

  widget.addSpacer(9);
  addRule(widget);
  widget.addSpacer(5);

  addTableRow(widget, "Rd", "Opponent", "Elo", "C", "Res", true);
  widget.addSpacer(5);

  const games = data.games.slice(-maximumRows);
  for (const game of games) {
    addTableRow(
      widget,
      String(game.round),
      compactName(game.opponent_name),
      game.opponent_rating > 0 ? String(game.opponent_rating) : "—",
      displayColor(game.color),
      displayResult(game),
      false,
      game.result_kind
    );
    widget.addSpacer(family === "large" ? 6 : 4);
  }

  if (games.length === 0) {
    const empty = widget.addText("No results yet");
    empty.font = Font.italicSystemFont(11);
    empty.textColor = new Color("9ca3af");
  }

  widget.addSpacer();
  const footer = widget.addText(
    `${stale ? "Offline cache" : "Updated"} • refresh ~${stale ? 10 : REFRESH_MINUTES} min`
  );
  footer.font = Font.systemFont(10);
  footer.textColor = new Color("6b7280");
  footer.rightAlignText();

  return widget;
}

function addTableRow(widget, round, opponent, rating, color, result, heading, resultKind) {
  const row = widget.addStack();
  row.centerAlignContent();

  addCell(row, round, 27, heading, "left");
  addCell(row, opponent, opponentWidth, heading, "left");
  addCell(row, rating, 42, heading, "right");
  addCell(row, color, 25, heading, "center");
  addCell(row, result, 31, heading, "right", resultColor(resultKind));
}

function addCell(row, value, width, heading, alignment, color) {
  const cell = row.addStack();
  if (width > 0) cell.size = new Size(width, 0);

  if (alignment !== "left") cell.addSpacer();
  const text = cell.addText(value);
  text.font = heading ? Font.boldSystemFont(11) : Font.semiboldSystemFont(13);
  text.textColor = color || (heading ? new Color("9ca3af") : new Color("e5e7eb"));
  text.lineLimit = 1;
  text.minimumScaleFactor = 0.7;

  if (alignment === "center") cell.addSpacer();
  if (alignment === "right") text.rightAlignText();
}

function addRule(widget) {
  const rule = widget.addStack();
  rule.size = new Size(0, 1);
  rule.backgroundColor = new Color("374151");
}

function compactName(name) {
  const parts = name.split(",");
  return parts.length > 1 ? `${parts[0]}, ${parts[1].trim().charAt(0)}.` : name;
}

function displayResult(game) {
  if (game.result_kind === "win") return "1";
  if (game.result_kind === "draw") return "½";
  if (game.result_kind === "loss") return "0";
  return game.result || "–";
}

function displayColor(color) {
  if (color === "white") return "♙";
  if (color === "black") return "♟";
  return "—";
}

function resultColor(kind) {
  if (kind === "win") return new Color("34d399");
  if (kind === "draw") return new Color("fbbf24");
  if (kind === "loss") return new Color("f87171");
  return new Color("e5e7eb");
}
