(subject, payload) => {
  this.hostServices.kv.set('hello', payload);
  this.hostServices.kv.delete('hello');

  this.hostServices.kv.set('hello2', payload);
  return this.hostServices.kv.get('hello2');
};
