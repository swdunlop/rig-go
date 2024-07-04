// This module comes from a CDN via the index.html script tags.
declare var MessagePack: {
  decode(input: Uint8Array): any;
  encode(input: any): Uint8Array;
};

interface Future {
  resolve: (value: any) => void;
  reject: (failure: Failure) => void;
}

interface Failure {
  code: number;
  msg: string;
}

class MRPC {
  ws: WebSocket;
  pending: Map<string, Future> = new Map();
  constructor() {
    this.ws = new WebSocket("/mrpc");
    this.ws.onopen = () => {
      console.log("connected");
    };
    this.ws.onmessage = async (e) => {
      const data = new Uint8Array(await e.data.arrayBuffer());
      const [id, method, output] = MessagePack.decode(data) as [
        string,
        string,
        any,
      ];
      const pending = this.pending.get(id);
      if (!pending) {
        console.error("no pending request for message", { id, method, output });
        return;
      }
      if (method == "succ") {
        pending.resolve(output);
      } else if (method == "fail") {
        pending.reject(output as Failure);
      } else {
        console.error("unknown method", { id, method, output });
      }
    };
  }

  call<I, O>(functionName: string, input: I): Promise<O> {
    const id = Math.random().toString(36).substring(7);
    const msg = MessagePack.encode([id, `call`, functionName, input]);
    this.ws.send(msg);
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
    });
  }

  bind<I, O>(method: string): (input: I) => Promise<O> {
    return (input: I) => this.call(method, input);
  }
}

const mrpc = new MRPC();
const printf = mrpc.bind<
  {
    msg: string;
    info: any[];
  },
  {
    str: string;
  }
>("printf");

mrpc.ws.onopen = () => {
  printf({
    msg: "Hello, %s!",
    info: ["world"],
  })
    .then(console.log)
    .catch(console.error);
};
