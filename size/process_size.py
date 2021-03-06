import MySQLdb
import datetime
import glob
import httplib2
import os
import pickle
import socket
import stat
import struct
import subprocess

from buildbot.interfaces import BuildSlaveTooOldError
from buildbot.process import buildstep
from buildbot.status.results import SUCCESS, WARNINGS, FAILURE

_DEFAULT_PORT = 2004    # Default port of pickled messages to Graphite
_SERVER_ADDRESS = '23.236.55.44'
_NAME_PREFIX = 'size.'
_METADATA_URL = 'http://metadata/computeMetadata/v1/instance/attributes/'
_DB_HOST = '173.194.240.40'
_DB_USER = 'readwrite'
_DB_NAME = 'skia'

def _sanitizeGraphiteNames(string):
  return string.replace('.', '_')

class ProcessSize(buildstep.BuildStep):
  def __init__(self, workdir, **kwargs):
    buildstep.BuildStep.__init__(self, **kwargs)
    self.workdir = workdir

  def start(self):
    ts = int(self.getProperty('ts'))
    results = []
    sub = []
    subtype = ['text', 'data', 'bss']
    for f in glob.iglob(os.path.join(self.getProperty('builddir'),
        self.workdir, 'out', 'Release', 'libskia*.a')):
      results.append((
          _NAME_PREFIX + _sanitizeGraphiteNames(os.path.basename(f)),
          (ts, os.stat(f).st_size)))

      proc = subprocess.Popen(['/usr/bin/size', f], stdout=subprocess.PIPE)
      out, err = proc.communicate()
      if not err and out:
        lines = out.split('\n')[1:]
        for l in lines:
          cols = l.strip().split('\t')
          if len(cols) != 6:
            continue
          o = cols[-1]
          o = o[:o.find('.o')]
          for i in range(len(subtype)):
	    if '0' == cols[i].strip():
              continue
            sub.append((
                _NAME_PREFIX + '%s.%s' % (o, subtype[i]),
                (ts, int(cols[i]))))

    if not len(results):
      self.step_status.setText('No libskia after build.')
      self.finished(WARNING)

    try:
      sock = socket.socket()
      sock.connect((_SERVER_ADDRESS, _DEFAULT_PORT))
      valid_events = results + sub
      for start in range(0, len(valid_events), 100):
        message = pickle.dumps(valid_events[start:start+100])
        header = struct.pack('!L', len(message))
        sock.sendall(header + message)
    except Exception:
      self.step_status.setText('Failed sending stats to Graphite.')
      self.finished(FAILURE)
    
    http = httplib2.Http()
    resp, passwd = http.request(uri=_METADATA_URL+'readwrite', method='GET',
        headers={'Metadata-Flavor': 'Google'})
    if resp == 500:
      self.step_status.setText('Failed to read Cloud SQL password.')
      self.finished(FAILURE)
    db = MySQLdb.connect(host=_DB_HOST, user=_DB_USER, passwd=passwd,
        db=_DB_NAME)
    sql = 'INSERT INTO sizes(ts,file,size) VALUES (%s,%s,%s)'
    cur = db.cursor()
    for f, d in results:
      t = datetime.datetime.utcfromtimestamp(d[0]).strftime('%Y-%m-%d %H:%M:%S')
      s = f[12:-2]
      z = int(d[1])
      if cur.execute(sql, (t, s, z)) < 0:
        self.step_status.setText('Failed to insert new values: (%s,%s,%s)' % (
            t, s, z))
        cur.close()
        db.close()
        self.finished(FAILURE)
    try:
      db.commit()
    except Exception:
      self.step_status.setText('Failed commit inserts.')
      db.rollback()
      cur.close()
      db.close()

    cur.close()
    db.close()
    self.step_status.setText(
        'Processed %d+%d Records: %s' % (len(results), len(sub), str(sub)))
    self.finished(SUCCESS)

