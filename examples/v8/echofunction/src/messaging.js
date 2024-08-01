(subject, payload) => {
  this.messaging.publish('hello.world', payload);

  var reqResp;
  var reqEx;

  var reqManyResp;
  var reqManyEx;

  try {
    reqResp = this.messaging.request('hello.world.request', payload);
    reqResp = String.fromCharCode(...reqResp)
  } catch (e) {
    reqEx = e;
  }

  // FIXME: request many is currently not supported
  // try {
  //   reqManyResp = []

  //   let responses = this.hostServices.messaging.requestMany('hello.world.request.many', payload)
  
  //   // responses is an array of Uint8Array... flatten it so we can use each value
  //   responses = Array.prototype.slice.call(responses)

  //   for (let i = 0; i < responses.length; i++) {
  //     reqManyResp.push(String.fromCharCode(...responses[i]))
  //   }
  // } catch (e) {
  //   reqManyEx = e;
  // }

  return {
    'hello.world.request': reqResp,
    'hello.world.request.ex': reqEx
//    'hello.world.request.many': reqManyResp,
//    'hello.world.request.many.ex': reqManyEx
  }
};
