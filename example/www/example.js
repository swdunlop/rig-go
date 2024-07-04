(() => {
  // example.ts
  var MRPC = class {
    ws;
    pending = /* @__PURE__ */ new Map();
    constructor() {
      this.ws = new WebSocket("/mrpc");
      this.ws.onopen = () => {
        console.log("connected");
      };
      this.ws.onmessage = async (e) => {
        const data = new Uint8Array(await e.data.arrayBuffer());
        const [id, method, output] = MessagePack.decode(data);
        const pending = this.pending.get(id);
        if (!pending) {
          console.error("no pending request for message", { id, method, output });
          return;
        }
        if (method == "succ") {
          pending.resolve(output);
        } else if (method == "fail") {
          pending.reject(output);
        } else {
          console.error("unknown method", { id, method, output });
        }
      };
    }
    call(functionName, input) {
      const id = Math.random().toString(36).substring(7);
      const msg = MessagePack.encode([id, `call`, functionName, input]);
      this.ws.send(msg);
      return new Promise((resolve, reject) => {
        this.pending.set(id, { resolve, reject });
      });
    }
    bind(method) {
      return (input) => this.call(method, input);
    }
  };
  var mrpc = new MRPC();
  var printf = mrpc.bind("printf");
  mrpc.ws.onopen = () => {
    printf({
      msg: "Hello, %s!",
      info: ["world"]
    }).then(console.log).catch(console.error);
  };
})();
