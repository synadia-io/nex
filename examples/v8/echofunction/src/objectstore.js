(subject, payload) => {
  this.objectStore("testObjBucket").put('hello', payload);
  this.objectStore("testObjBucket").delete('hello');

  this.objectStore("testObjBucket").put('hello2', payload);
  return {
    list: this.objectStore("testObjBucket").list(),
    hello2: String.fromCharCode(...this.objectStore("testObjBucket").get('hello2'))
  }
};
