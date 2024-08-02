(subject, payload) => {
  this.kv("testBucket").set('hello', payload);
  this.kv("testBucket").delete('hello');

  this.kv("testBucket").set('hello2', payload);
  return {
    keys: this.kv("testBucket").keys(),
    hello2: String.fromCharCode(...this.kv("testBucket").get('hello2'))
  }
};
