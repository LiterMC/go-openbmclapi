using System;
using System.Collections.Generic;
using System.Text;
using Microsoft.Deployment.WindowsInstaller;

namespace CustomAction {
	public class CustomAction {
		[CustomAction]
		public static ActionResult SetupCluster(Session session) {
			session.Log("Begin SetupCluster");

			CustomActionData data = session.CustomActionData;

			session.Message(InstallMessage.Warning,
				new Record(new string[]{
					string.Format("Data {0}", session.CustomActionData)
				}));

			return ActionResult.Success;
		}
	}
}
