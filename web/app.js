const term = new Terminal({
  cursorBlink: true,
  fontFamily: "monospace",
  fontSize: 13,
  theme: { background: "#000000" }
});

term.open(document.getElementById("terminal"));
term.focus();

term.onData(data => {
  window.goWrite(data);
});

function connect() {
  window.goConnect();
}

function termWrite(data) {
  term.write(data);
}
