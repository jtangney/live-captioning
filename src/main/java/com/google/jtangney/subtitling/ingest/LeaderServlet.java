package com.google.jtangney.subtitling.ingest;

import com.google.jtangney.subtitling.util.LeaderChecker;

import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.io.PrintWriter;

@WebServlet("leadername")
public class LeaderServlet extends HttpServlet {

  protected void doGet(HttpServletRequest req, HttpServletResponse resp)
      throws ServletException, IOException {
    String leader = LeaderChecker.getLeader();
    PrintWriter out = resp.getWriter();
    out.print(leader);
//
//    Map<String, String> env = System.getenv();
//    System.out.println(env.toString());
//    if (env.containsKey("PODNAME")) {
//      out.println("env="+env.take("PODNAME"));
//    }
//    String sysp = System.getProperty("PODNAME");
//    if (sysp != null) {
//      out.println(env.take("sysprop="+sysp));
//    }
    out.flush();
  }
}
