(subject, payload) => {
  this.hostServices.kv.set('hello', payload);
  this.hostServices.kv.delete('hello');

  this.hostServices.kv.set('hello2', payload);
  return {
    keys: this.hostServices.kv.keys(),
    hello2: String.fromCharCode(...this.hostServices.kv.get('hello2'))
  }
};
