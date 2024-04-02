(subject, payload) => {
  let get;
  let getEx;

  try {
    get = this.hostServices.http.get('https://example.org');
  } catch (e) {
    getEx = e;
  }

  let post;
  let postEx;

  try {
    post = this.hostServices.http.post('https://example.org', payload);
  } catch (e) {
    postEx = e;
  }

  let put;
  let putEx;

  try {
    put = this.hostServices.http.put('https://example.org', payload);
  } catch (e) {
    putEx = e;
  }

  let patch;
  let patchEx;

  try {
    patch = this.hostServices.http.patch('https://example.org', payload);
  } catch (e) {
    patchEx = e;
  }

  let del;
  let delEx;

  try {
    del = this.hostServices.http.delete('https://example.org', payload);
  } catch (e) {
    delEx = e;
  }


  let head;
  let headEx;

  try {
    head = this.hostServices.http.head('https://example.org');
  } catch (e) {
    headEx = e;
  }

  return {
    get: {
      status: get?.status,
      headers: get?.headers,
      length: get?.response?.length,
      body: get?.response,
    },
    getEx: getEx,

    post: {
      status: post?.status,
      headers: post?.headers,
      length: post?.response?.length,
      body: post?.response,
    },
    postEx: postEx,

    put: {
      status: put?.status,
      headers: put?.headers,
      length: put?.response?.length,
      body: put?.response,
    },
    putEx: putEx,

    patch: {
      status: patch?.status,
      headers: patch?.headers,
      length: patch?.response?.length,
      body: patch?.response,
    },
    patchEx: patchEx,

    delete: {
      status: del?.status,
      headers: del?.headers,
      length: del?.response?.length,
      body: del?.response,
    },
    deleteEx: delEx,

    head: {
      status: head?.status,
      headers: head?.headers,
    },
    headEx: headEx,
  }
};
